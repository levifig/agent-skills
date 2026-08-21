package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	seedNativeOpenCodeBuildFixture(t, root)
	seedNativeClaudeCodeBuildFixture(t, root)
	for _, staleFile := range []string{
		filepath.Join(root, "plugins", "loaf", "stale.txt"),
		filepath.Join(root, "dist", "opencode", "stale.txt"),
		filepath.Join(root, "dist", "cursor", "stale.txt"),
		filepath.Join(root, "dist", "codex", "stale.txt"),
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
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "claude-code", "opencode", "cursor", "codex", "amp", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	for _, staleFile := range []string{
		filepath.Join(root, "plugins", "loaf", "stale.txt"),
		filepath.Join(root, "dist", "opencode", "stale.txt"),
		filepath.Join(root, "dist", "cursor", "stale.txt"),
		filepath.Join(root, "dist", "codex", "stale.txt"),
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
		filepath.Join(root, "dist", "cursor", hookCatalogFile),
		filepath.Join(root, "dist", "codex", ".codex", "hooks.json"),
		filepath.Join(root, "dist", "codex", hookCatalogFile),
		filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
	}
	// The hook catalog is emitted only for the targets whose hook files
	// reconcile per entry. OpenCode and Amp ship plugins, and Claude Code's
	// hooks live in the plugin bundle; none of them are in this scope.
	for _, target := range []string{"claude-code", "opencode", "amp"} {
		path := filepath.Join(nativeBuildTargetOutputDir(root, target), hookCatalogFile)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Stat(%s) = %v, want no hook catalog for %s", path, err, target)
		}
	}
	for target, adapter := range map[string]string{
		"claude-code": "claude-session-start-v1",
		"opencode":    "opencode-plugin-v1",
		"cursor":      "cursor-session-start-v1",
		"codex":       "codex-session-start-v1",
		"amp":         "amp-plugin-v1",
	} {
		manifestPath := filepath.Join(nativeBuildTargetOutputDir(root, target), ".loaf-target-manifest.json")
		manifest := readBuildJSON(t, manifestPath)
		if manifest["version"] != float64(1) || manifest["target"] != target || manifest["package_version"] != "9.8.7-test.1" || manifest["capability_contract_version"] != float64(3) {
			t.Fatalf("%s manifest = %#v, want strict target metadata", target, manifest)
		}
		adapters, ok := manifest["adapters"].([]any)
		if !ok || len(adapters) != 1 || adapters[0] != adapter {
			t.Fatalf("%s manifest adapters = %#v, want %q", target, manifest["adapters"], adapter)
		}
		artifacts, ok := manifest["artifacts"].([]any)
		// Codex's only surface is the shared hooks file, which is reconciled per
		// entry and therefore on no manifest at all — so its manifest is the
		// managed instruction and nothing else.
		wantArtifacts := 2
		if target == "codex" {
			wantArtifacts = 1
		}
		if !ok || len(artifacts) < wantArtifacts {
			t.Fatalf("%s manifest artifacts = %#v, want at least %d", target, manifest["artifacts"], wantArtifacts)
		}
		var instruction map[string]any
		for _, rawArtifact := range artifacts {
			artifact := rawArtifact.(map[string]any)
			if artifact["id"] == "managed-instructions" {
				instruction = artifact
			}
			// No manifest names a target's shared hooks file, under the retired
			// kind or any other: a whole-file row for it is the file-level verdict
			// entry-level reconciliation replaced.
			if kind, _ := artifact["kind"].(string); kind == obsoleteHookProjectionKind {
				t.Fatalf("%s manifest artifact = %#v, want the retired hook-projection kind gone", target, artifact)
			}
			switch artifact["destination"] {
			case "hooks.json", "hooks/hooks.json":
				if target != "claude-code" {
					t.Fatalf("%s manifest artifact = %#v, want the reconciled hooks file off the manifest", target, artifact)
				}
			}
		}
		if instruction == nil {
			t.Fatalf("%s manifest has no managed instruction artifact", target)
		}
		if instruction["id"] != "managed-instructions" || instruction["kind"] != "instruction" || instruction["destination"] != "project-instructions" || len(instruction["sha256"].(string)) != 64 {
			t.Fatalf("%s managed instruction artifact = %#v", target, instruction)
		}
	}
}

func TestRunnerBuildTargetCodexRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "codex"})
	if err != nil {
		t.Fatalf("build --target codex error = %v\n%s", err, stdout.String())
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
	if !strings.Contains(sharedSkill, "{{IMPLEMENT_CMD}}") {
		t.Fatalf("shared skill = %q, want authored tokens left unsubstituted", sharedSkill)
	}
	if strings.Contains(sharedSkill, "Run /implement now.") {
		t.Fatalf("shared skill = %q, must not rewrite {{IMPLEMENT_CMD}}", sharedSkill)
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
	if !strings.Contains(codexSkill, "{{IMPLEMENT_CMD}}") {
		t.Fatalf("codex skill = %q, want authored tokens left unsubstituted", codexSkill)
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "codex", "skills", "demo", "templates", "session.md")), "{{RESUME_CMD}}") {
		t.Fatalf("shared template was not copied without prose substitution")
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
	if !strings.Contains(hooksJSON, `"SessionStart": [`) {
		t.Fatalf("hooks.json = %q, want current Codex SessionStart hooks schema", hooksJSON)
	}
	var hooks nativeCodexHooksJSON
	if err := json.Unmarshal([]byte(hooksJSON), &hooks); err != nil {
		t.Fatalf("Unmarshal(codex hooks) error = %v\n%s", err, hooksJSON)
	}
	if len(hooks.Hooks.SessionStart) != 1 || hooks.Hooks.SessionStart[0].Matcher != "startup|resume|clear|compact" {
		t.Fatalf("codex hooks = %q, want one SessionStart matcher group", hooksJSON)
	}
	if len(hooks.Hooks.SessionStart[0].Hooks) != 1 || hooks.Hooks.SessionStart[0].Hooks[0].Command != "{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook" || hooks.Hooks.SessionStart[0].Hooks[0].CommandWindows != "{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook" {
		t.Fatalf("codex hooks = %q, want unresolved path-pinned adapter command", hooksJSON)
	}
	if strings.Contains(hooksJSON, "version") || strings.Contains(hooksJSON, "loaf check --hook") {
		t.Fatalf("codex hooks = %q, want no legacy version or enforcement handlers", hooksJSON)
	}
	if strings.Contains(hooksJSON, "workflow-pre-merge") || strings.Contains(hooksJSON, "detect-linear-magic") {
		t.Fatalf("hooks.json = %q, want only SessionStart context hook", hooksJSON)
	}
	catalog, err := readHookCatalog(filepath.Join(root, "dist", "codex"))
	if err != nil {
		t.Fatalf("readHookCatalog(codex) error = %v", err)
	}
	if len(catalog.Entries) != 1 || catalog.Entries[0].Event != "SessionStart" || catalog.Entries[0].HookID != "session-start-loaf" {
		t.Fatalf("codex hook catalog = %#v, want the SessionStart identity", catalog.Entries)
	}
}

func TestRunnerBuildTargetAmpRunsNativePluginTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
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
	for _, want := range []string{"loaf build", "shared skills intermediate", "amp", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleAmpFile); !os.IsNotExist(err) {
		t.Fatalf("stale amp file stat = %v, want target output reset", err)
	}

	ampSkill := readBuildFileString(t, filepath.Join(root, "dist", "amp", "skills", "demo", "SKILL.md"))
	if !strings.Contains(ampSkill, "version: 9.8.7-test.1") || !strings.Contains(ampSkill, "{{IMPLEMENT_CMD}}") {
		t.Fatalf("amp skill = %q, want version injection without command substitution", ampSkill)
	}
	plugin := readBuildFileString(t, filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"))
	for _, want := range []string{
		"@version 9.8.7-test.1",
		"import type { PluginAPI } from '@ampcode/plugin';",
		"export default function (amp: PluginAPI)",
		"interface AmpToolCallEvent",
		"toolUseID: string;",
		"thread: { id: string };",
		"status: 'done' | 'error' | 'cancelled';",
		"function normalizeAmpToolName(toolName: string): string",
		"case 'shell_command':",
		"return 'Bash';",
		"case 'create_file':",
		"return 'Write';",
		"case 'edit_file':",
		"case 'apply_patch':",
		"return 'Edit';",
		"amp.helpers.shellCommandFromToolCall(event)",
		"normalizedInput.cwd = shellCommand.dir",
		"amp.on('tool.call', async (event: AmpToolCallEvent) =>",
		"amp.on('tool.result', async (event: AmpToolResultEvent) =>",
		"return { action: 'reject-and-continue', message: result.stderr }",
		"return { action: 'allow' }",
		"raw: rawInput",
		`"command": "loaf check --hook check-secrets"`,
		`"command": "cat \"$LOAF_PLUGIN_DIR/hooks/instructions/pre-merge.md\""`,
		`const postToolHooks: Record<string, HookEntry[]> = {`,
		`"script": "post-tool/kb-staleness-nudge.sh"`,
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("amp plugin = %q, want %q", plugin, want)
		}
	}
	if strings.Contains(plugin, "@i-know-the-amp-plugin-api-is-wip") || strings.Contains(plugin, "call.toolName") {
		t.Fatalf("amp plugin = %q, want documented plugin API without WIP header or undefined call reference", plugin)
	}
	for _, unwanted := range []string{
		"session.start",
		"agent.start",
		"arguments",
		"$HOME/.amp/plugins",
		"const sessionHooks",
		"declare module '@ampcode/plugin'",
		"detect-linear-magic",
		"loaf journal log --detect-linear",
	} {
		if strings.Contains(plugin, unwanted) {
			t.Fatalf("amp plugin = %q, must not contain obsolete projection %q", plugin, unwanted)
		}
	}
	if count := strings.Count(plugin, "return { action: 'allow' }"); count != 1 {
		t.Fatalf("amp plugin allow returns = %d, want tool.call only", count)
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "amp", "plugins", "loaf.js")); !os.IsNotExist(err) {
		t.Fatalf("amp loaf.js stat = %v, want TypeScript project plugin only", err)
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "amp", "skills", "demo", "templates", "session.md")), "{{RESUME_CMD}}") {
		t.Fatalf("amp shared template was not copied without prose substitution")
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "amp", ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("amp hooks stat = %v, want Amp plugin target without Codex hooks", err)
	}
}

func TestRunnerBuildTargetAmpRegistersDelegatedAgents(t *testing.T) {
	plugin := buildAmpPluginForDelegateTests(t)
	for _, want := range []string{
		`// @amp-agent-mode {"key":"loaf-medium","label":"Loaf Medium"}`,
		`// @amp-agent-mode {"key":"loaf-ultra","label":"Loaf Ultra"}`,
		"amp.on('tool.call'",
		"amp.on('tool.result'",
		"amp.createAgent",
		"amp.registerAgentMode",
		"amp.registerTool",
		"name: 'loaf-medium'",
		"name: 'loaf-ultra'",
		"model: 'openai/gpt-5.6-sol'",
		"reasoningEffort: 'medium'",
		"reasoningEffort: 'xhigh'",
		"key: 'loaf-medium'",
		"key: 'loaf-ultra'",
		"name: 'grok-implementation-agent'",
		"model: 'xai/grok-4.6'",
		"reasoningEffort: 'high'",
		"features: ['fast']",
		"name: 'delegate_grok_implementation'",
		"name: 'luna-review-agent'",
		"model: 'openai/gpt-5.6-luna'",
		"name: 'delegate_luna_review'",
		"name: 'sol-oracle-agent'",
		"name: 'consult_oracle'",
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("amp plugin missing %q", want)
		}
	}
	for _, banned := range []string{
		`// @amp-agent-mode {"key":"grok-impl","label":"Grok Implement"}`,
		`// @amp-agent-mode {"key":"luna-review","label":"Luna Review"}`,
		"key: 'grok-impl'",
		"key: 'luna-review'",
	} {
		if strings.Contains(plugin, banned) {
			t.Fatalf("amp plugin must not register picker mode %q", banned)
		}
	}
}

func TestRunnerBuildTargetAmpConstrainsDelegatedAgents(t *testing.T) {
	plugin := buildAmpPluginForDelegateTests(t)
	grokTools := extractAmpDelegateToolList(t, plugin, "GROK_IMPL_TOOLS")
	lunaTools := extractAmpDelegateToolList(t, plugin, "LUNA_REVIEW_TOOLS")
	if got, want := grokTools, []string{"Read", "finder", "shell_command", "shell_command_status", "apply_patch", "view_media"}; !sameStringSlice(got, want) {
		t.Fatalf("grok tools = %#v, want finite local coding allowlist %#v", got, want)
	}
	if got, want := lunaTools, []string{"Read", "finder"}; !sameStringSlice(got, want) {
		t.Fatalf("luna tools = %#v, want read/search only %#v", got, want)
	}
	for _, banned := range []string{
		`include: "all"`,
		`include: 'all'`,
		"tools: 'all'",
	} {
		if strings.Contains(plugin, banned) {
			t.Fatalf("amp plugin contains banned capability expansion %q", banned)
		}
	}
	for _, forbidden := range []string{"web_search", "read_web_page", "librarian", "skill", "oracle", "Task", "mcp__"} {
		for _, tools := range [][]string{grokTools, lunaTools} {
			for _, tool := range tools {
				if tool == forbidden {
					t.Fatalf("delegate allowlist includes forbidden tool %q", tool)
				}
			}
		}
	}
	lunaBlock := extractAmpDelegateBlock(t, plugin, "const lunaReviewer", "const solOracle")
	for _, banned := range []string{"shell_command", "apply_patch", "create_file", "edit_file"} {
		if strings.Contains(lunaBlock, banned) {
			t.Fatalf("luna agent block contains banned capability %q", banned)
		}
	}
	reviewExecute := extractAmpDelegateBlock(t, plugin, "name: 'delegate_luna_review'", "export default function")
	for _, want := range []string{
		"required: ['workdir', 'diff', 'prompt']",
		"requireNonEmptyString(input.diff, 'diff')",
		"Diff supplied by the delegating agent",
	} {
		if !strings.Contains(reviewExecute, want) {
			t.Fatalf("luna review tool missing %q", want)
		}
	}
	if strings.Contains(reviewExecute, "git ") || strings.Contains(reviewExecute, "execFile") || strings.Contains(reviewExecute, "shell_command") {
		t.Fatal("luna review execute must not run Git or shell")
	}
	for _, want := range []string{
		"requireNonEmptyString(input.prompt, 'prompt')",
		"workdir must be an absolute path",
		"workdir must be an existing directory",
		"realpathSync",
		"isAbsolute",
		"parentThreadID: ctx.thread.id",
		"executor: 'local'",
		"Do not commit, push, merge, rewrite history",
		"Workdir is routing context, not a sandbox",
		"amp plugins show-agent-options",
		"Do not substitute",
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("amp plugin missing constraint %q", want)
		}
	}
}

func TestRunnerBuildTargetAmpDocumentsDelegateDefaults(t *testing.T) {
	plugin := buildAmpPluginForDelegateTests(t)
	for _, want := range []string{
		"Selectable modes are loaf-medium and loaf-ultra only",
		"Grok, Luna, and the",
		"pinned Sol oracle are tools, not picker modes",
		"Built-in Amp medium cannot be",
		"rewritten; operators use Loaf Medium instead",
		"no local fallback",
		"no silent fallback",
		"key: 'loaf-medium'",
		"key: 'loaf-ultra'",
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("amp plugin missing default-policy language %q", want)
		}
	}
}

func TestAmpDelegateGuidanceCoversIsolationSnapshotsPreflightAndMigration(t *testing.T) {
	root := testRepositoryRoot(t)
	guidance := readBuildFileString(t, filepath.Join(root, "content", "skills", "orchestration", "references", "amp-delegates.md"))
	for _, want := range []string{
		"Workdir is routing context, not a sandbox",
		"Loaf-started isolated worktree",
		"caller-prepared",
		"Loaf itself must not execute Git",
		"amp plugins show-agent-options",
		"loaf-medium",
		"loaf-ultra",
		"delegate_grok_implementation",
		"delegate_luna_review",
		"consult_oracle",
		"not selectable picker modes",
		"~/.config/amp/plugins/delegated-agents.ts",
		"Loaf must never overwrite or delete",
		"no silent fallback",
	} {
		if !strings.Contains(guidance, want) {
			t.Fatalf("amp delegate guidance missing %q", want)
		}
	}
}

func TestAmpDelegatedAgentDuplicateRegistrationFailsClosed(t *testing.T) {
	plugin := buildAmpPluginForDelegateTests(t)
	if !strings.Contains(plugin, "function registerPinnedAgentMode") || !strings.Contains(plugin, "function registerPinnedTool") {
		t.Fatal("amp plugin must wrap registerAgentMode and registerTool failures")
	}
	result := runAmpDelegateHarness(t, plugin, map[string]any{
		"case": "duplicate-registration",
	})
	if result.OK {
		t.Fatalf("duplicate registration succeeded: %#v", result)
	}
	assertAmpDelegateCompatibilityFailure(t, result.Error, "already registered")
}

func TestAmpDelegatedAgentRuntimeContract(t *testing.T) {
	plugin := buildAmpPluginForDelegateTests(t)
	workdir := realpath(t, t.TempDir())
	filePath := filepath.Join(workdir, "notes.txt")
	writeFile(t, filePath, "notes\n")

	happy := runAmpDelegateHarness(t, plugin, map[string]any{
		"case":    "happy",
		"workdir": workdir,
		"prompt":  "implement LOAF-61",
		"diff":    "diff --git a/notes.txt b/notes.txt\n",
	})
	if !happy.OK {
		t.Fatalf("happy-path delegate error = %s", happy.Error)
	}
	if happy.Grok.Model != "xai/grok-4.6" || happy.Grok.Reasoning != "high" {
		t.Fatalf("grok pin = %#v", happy.Grok)
	}
	if happy.Luna.Model != "openai/gpt-5.6-luna" || happy.Luna.Reasoning != "xhigh" {
		t.Fatalf("luna pin = %#v", happy.Luna)
	}
	if !sameStringSlice(happy.Grok.Tools, []string{"Read", "finder", "shell_command", "shell_command_status", "apply_patch", "view_media"}) {
		t.Fatalf("grok tools = %#v", happy.Grok.Tools)
	}
	if !sameStringSlice(happy.Luna.Tools, []string{"Read", "finder"}) {
		t.Fatalf("luna tools = %#v", happy.Luna.Tools)
	}
	if happy.Grok.Executor != "local" || happy.Luna.Executor != "local" {
		t.Fatalf("executors = grok %q luna %q, want local", happy.Grok.Executor, happy.Luna.Executor)
	}
	if happy.Grok.ParentThreadID != "T-parent" || happy.Luna.ParentThreadID != "T-parent" {
		t.Fatalf("parentThreadIDs = grok %q luna %q", happy.Grok.ParentThreadID, happy.Luna.ParentThreadID)
	}
	if !strings.Contains(happy.Grok.Prompt, "Workspace: "+workdir) || !strings.Contains(happy.Grok.Prompt, "implement LOAF-61") {
		t.Fatalf("grok prompt = %q", happy.Grok.Prompt)
	}
	if !strings.Contains(happy.Luna.Prompt, "Diff supplied by the delegating agent:") || !strings.Contains(happy.Luna.Prompt, "diff --git a/notes.txt") {
		t.Fatalf("luna prompt = %q", happy.Luna.Prompt)
	}
	if happy.Luna.GitInvoked || happy.Luna.ShellInvoked {
		t.Fatal("luna review must not invoke Git or shell")
	}

	for _, tc := range []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{name: "empty prompt", payload: map[string]any{"case": "invalid-prompt", "workdir": workdir, "prompt": "   "}, want: "prompt must be a non-empty string"},
		{name: "relative workdir", payload: map[string]any{"case": "invalid-workdir", "workdir": "relative", "prompt": "implement"}, want: "workdir must be an absolute path"},
		{name: "file workdir", payload: map[string]any{"case": "invalid-workdir", "workdir": filePath, "prompt": "implement"}, want: "workdir must be an existing directory"},
		{name: "empty review diff", payload: map[string]any{"case": "missing-diff", "workdir": workdir, "prompt": "review", "diff": " "}, want: "diff must be a non-empty string"},
	} {
		result := runAmpDelegateHarness(t, plugin, tc.payload)
		if result.OK {
			t.Fatalf("%s succeeded: %#v", tc.name, result)
		}
		if !strings.Contains(result.Error, tc.want) {
			t.Fatalf("%s error = %q, want %q", tc.name, result.Error, tc.want)
		}
	}

	createFail := runAmpDelegateHarness(t, plugin, map[string]any{"case": "create-failure"})
	if createFail.OK {
		t.Fatalf("create failure succeeded: %#v", createFail)
	}
	assertAmpDelegateCompatibilityFailure(t, createFail.Error, "createAgent unavailable")

	runFail := runAmpDelegateHarness(t, plugin, map[string]any{"case": "run-failure", "workdir": workdir, "prompt": "implement"})
	if runFail.OK {
		t.Fatalf("run failure succeeded: %#v", runFail)
	}
	assertAmpDelegateCompatibilityFailure(t, runFail.Error, "agent.run unavailable")
}

func TestAmpDelegateGuidanceIsSharedSkillNotPerTargetRewrite(t *testing.T) {
	root := testRepositoryRoot(t)
	authored := readBuildFileString(t, filepath.Join(root, "content", "skills", "orchestration", "references", "amp-delegates.md"))
	if !strings.Contains(authored, "does not change Claude Code, Codex, Cursor, or OpenCode behavior") {
		t.Fatal("authored Amp guidance must state that other harnesses keep their behavior")
	}
	for _, path := range []string{
		filepath.Join(root, "dist", "codex", "skills", "orchestration", "references", "amp-delegates.md"),
		filepath.Join(root, "dist", "cursor", "skills", "orchestration", "references", "amp-delegates.md"),
		filepath.Join(root, "dist", "opencode", "skills", "orchestration", "references", "amp-delegates.md"),
		filepath.Join(root, "plugins", "loaf", "skills", "orchestration", "references", "amp-delegates.md"),
	} {
		body := readBuildFileString(t, path)
		if body != authored {
			t.Fatalf("%s rewrote Amp guidance instead of copying the shared skill bytes", path)
		}
	}
}

func TestAmpTypeScriptValidationUsesLocalAmbientDeclarations(t *testing.T) {
	types := nativeBuildTypeScriptAmbientTypes()
	if !strings.Contains(types, "declare module '@ampcode/plugin'") {
		t.Fatal("TypeScript validation must declare a local Amp plugin ambient module")
	}
	if strings.Contains(types, "from '@ampcode/plugin'") || strings.Contains(types, "node_modules/@ampcode/plugin") {
		t.Fatal("TypeScript validation must not import the installed Amp runtime package")
	}
	if !strings.Contains(types, "This ambient declaration is a local typecheck surface, not the installed Amp runtime API.") {
		t.Fatal("TypeScript ambient types must disclose they are not the installed Amp runtime")
	}
}

func TestRunnerBuildTargetCursorRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
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
	for _, want := range []string{"loaf build", "shared skills intermediate", "cursor", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleCursorFile); !os.IsNotExist(err) {
		t.Fatalf("stale cursor file stat = %v, want target output reset", err)
	}

	cursorSkill := readBuildFileString(t, filepath.Join(root, "dist", "cursor", "skills", "demo", "SKILL.md"))
	if !strings.Contains(cursorSkill, "version: 9.8.7-test.1") || !strings.Contains(cursorSkill, "{{IMPLEMENT_CMD}}") {
		t.Fatalf("cursor skill = %q, want version injection without command substitution", cursorSkill)
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
		`"command": "loaf check --hook ephemeral-provenance"`,
		`"command": "loaf check --hook github-account"`,
		`"command": "loaf check --hook validate-push --advisory"`,
		`"command": "loaf check --hook workflow-pre-pr --advisory"`,
		`"command": "cat \"$HOME/.cursor/hooks/instructions/pre-merge.md\""`,
		`"postToolUse": [`,
		`"command": "bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh"`,
		`"sessionStart": [`,
		`"command": "loaf journal context --from-hook --cursor-hook"`,
	} {
		if !strings.Contains(hooksJSON, want) {
			t.Fatalf("cursor hooks.json = %q, want %q", hooksJSON, want)
		}
	}
	catalog, err := readHookCatalog(filepath.Join(root, "dist", "cursor"))
	if err != nil {
		t.Fatalf("readHookCatalog(cursor) error = %v", err)
	}
	generated := 0
	for _, entries := range testHookEventEntries(t, []byte(hooksJSON)) {
		generated += len(entries)
	}
	if len(catalog.Entries) != generated {
		t.Fatalf("cursor hook catalog has %d entries, want one per generated hooks.json entry (%d)", len(catalog.Entries), generated)
	}
	if _, ok := catalog.cohortHookIDs("0.2.20"); !ok {
		t.Fatal("cursor hook catalog carries no 0.2.20 absorption cohort")
	}
}

func TestRunnerBuildTargetOpenCodeRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
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
		"{{IMPLEMENT_CMD}}",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("opencode command = %q, want %q", command, want)
		}
	}
	workflowCommand := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "commands", "workflow-only.md"))
	for _, want := range []string{
		"description: Workflow-only skill without an OpenCode sidecar.",
		"version: 9.8.7-test.1",
		"{{IMPLEMENT_CMD}}",
	} {
		if !strings.Contains(workflowCommand, want) {
			t.Fatalf("opencode workflow-only command = %q, want %q", workflowCommand, want)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "opencode", "commands", "reference-only.md")); !os.IsNotExist(err) {
		t.Fatalf("reference-only command stat = %v, want no OpenCode command for non-invocable skill", err)
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
		"type OpenCodeClient = {",
		"get(input: { path: { id: string } }): Promise<{ data?: { parentID?: string } }>",
		"client.session.get({ path: { id: sessionID } })",
		"!response.data || typeof response.data !== 'object'",
		"if ('parentID' in data && data.parentID !== undefined)",
		"'tool.execute.before': async (input: { tool: string; sessionID: string; callID: string }, output: { args: unknown }) =>",
		"'tool.execute.after': async (input: { tool: string; sessionID: string; callID: string; args: unknown }, output: { title?: string; output?: string; metadata?: unknown }) =>",
		"normalizeOpenCodeToolName(input.tool)",
		"case 'bash':",
		"case 'edit':",
		"case 'write':",
		"serializeHookPayload(toolName, toolInput, { input, output })",
		"if (hook.id === 'detect-linear-magic' && !(await isOpenCodeRootSession(client, input.sessionID))) continue;",
		"'experimental.chat.system.transform': async (input: { sessionID?: string; model?: unknown }, output: { system: string[] }) =>",
		"runOpenCodeSessionHooks(sessionHooks.sessionstart, sessionID, 'system.transform', output.system)",
		"'experimental.session.compacting': async (input: { sessionID: string }, output: { context: string[]; prompt?: string }) =>",
		"runOpenCodeSessionHooks(sessionHooks.postcompact, sessionID, 'session.compacting', output.context)",
		"target: 'opencode'",
		"session_id: sessionID",
		"lifecycle_event: lifecycleEvent",
		"const stdout = result.stdout.trim()",
		"output.push(stdout)",
		`"command": "loaf check --hook check-secrets"`,
		`"command": "cat \"$LOAF_PLUGIN_DIR/hooks/instructions/pre-merge.md\""`,
		`"script": "post-tool/kb-staleness-nudge.sh"`,
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("opencode plugin = %q, want %q", plugin, want)
		}
	}
	for _, unwanted := range []string{
		"input?.tool?.name",
		"input?.tool?.input",
		"event.type === 'session.ended'",
		"event.type === 'context.compacting'",
		"'event': async",
		"sessionHooks.sessionend",
		"sessionHooks.precompact",
		"runtime_version",
		"harness_version",
	} {
		if strings.Contains(plugin, unwanted) {
			t.Fatalf("opencode plugin = %q, must not contain obsolete shape %q", plugin, unwanted)
		}
	}
	if readBuildFileString(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks", "post-tool", "kb-staleness-nudge.sh")) != "#!/bin/sh\necho opencode hook\n" {
		t.Fatalf("opencode hook copy mismatch")
	}
}

func TestRunnerBuildTargetClaudeCodeRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
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
		"{{IMPLEMENT_CMD}}",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("claude skill = %q, want %q", skill, want)
		}
	}
	if strings.Contains(skill, "/loaf:implement") {
		t.Fatalf("claude skill = %q, must not scope slash commands in skill bodies", skill)
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
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook ephemeral-provenance"`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook github-account"`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook validate-push --advisory"`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook workflow-pre-pr --advisory"`,
		`"command": "cat \"${CLAUDE_PLUGIN_ROOT}/hooks/instructions/pre-merge.md\""`,
		`"PostToolUse": [`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" task refresh"`,
		`"command": "bash ${CLAUDE_PLUGIN_ROOT}/hooks/kb-staleness-nudge.sh"`,
		`"SessionStart": [`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" journal context --from-hook --claude-code"`,
	} {
		if !strings.Contains(hooksJSON, want) {
			t.Fatalf("claude hooks.json = %q, want %q", hooksJSON, want)
		}
	}
	for _, reject := range []string{
		`check --hook check-secrets --advisory`,
		`bash ${CLAUDE_PLUGIN_ROOT}/hooks/.`,
	} {
		if strings.Contains(hooksJSON, reject) {
			t.Fatalf("claude hooks.json = %q, must not contain %q", hooksJSON, reject)
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

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: root,
	}.Run([]string{"build", "--target", "bogus"})
	if err == nil {
		t.Fatal("build --target bogus error = nil, want native target validation")
	}
	for _, want := range []string{"Unknown target bogus", "Valid targets: claude-code, opencode, cursor, codex, amp"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
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
	if !strings.Contains(err.Error(), "Build failed") {
		t.Fatalf("error = %v, want native build failure", err)
	}
	if !strings.Contains(stdout.String(), "Loaf launcher not found at bin/loaf") {
		t.Fatalf("stdout = %q, want target failure detail", stdout.String())
	}
}

func TestNativeBuildValidationRejectsMalformedJavaScript(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "dist", "opencode", "plugins"))
	writeFile(t, filepath.Join(root, "dist", "opencode", "plugins", "bad.js"), "function {\n")

	warnings, err := validateNativeBuildArtifacts(root, "opencode")
	if err == nil {
		t.Fatal("validateNativeBuildArtifacts error = nil, want malformed JavaScript failure")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none on JavaScript failure", warnings)
	}
	if !strings.Contains(err.Error(), "JavaScript validation failed") || !strings.Contains(err.Error(), "dist/opencode/plugins/bad.js") {
		t.Fatalf("error = %v, want JavaScript validation path", err)
	}
}

func TestNativeBuildValidationWarnsWhenTypeScriptToolMissingOutsideCI(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "dist", "opencode", "plugins"))
	writeFile(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"), "const ok: string = 'ok';\n")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CI", "")

	warnings, err := validateNativeBuildArtifacts(root, "opencode")
	if err != nil {
		t.Fatalf("validateNativeBuildArtifacts error = %v, want local warning only", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "TypeScript validation skipped") || !strings.Contains(warnings[0], "dist/opencode/plugins/hooks.ts") {
		t.Fatalf("warnings = %#v, want missing tsc warning with file path", warnings)
	}
}

func TestNativeBuildValidationRequiresTypeScriptToolInCI(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "dist", "opencode", "plugins"))
	writeFile(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"), "const ok: string = 'ok';\n")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CI", "true")

	warnings, err := validateNativeBuildArtifacts(root, "opencode")
	if err == nil {
		t.Fatal("validateNativeBuildArtifacts error = nil, want missing tsc CI failure")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none on CI failure", warnings)
	}
	if !strings.Contains(err.Error(), "TypeScript validation requires tsc in CI") {
		t.Fatalf("error = %v, want CI tsc requirement", err)
	}
}

func TestNativeBuildValidationRunsTypeScriptToolWhenPresent(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "dist", "opencode", "plugins"))
	writeFile(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"), "const ok: string = 'ok';\n")
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "tsc.log")
	writeFile(t, filepath.Join(bin, "tsc"), strings.Join([]string{
		"#!/bin/sh",
		`printf '%s\n' "$*" > "` + logPath + `"`,
		"exit 0",
		"",
	}, "\n"))
	if err := os.Chmod(filepath.Join(bin, "tsc"), 0o755); err != nil {
		t.Fatalf("Chmod(tsc) error = %v", err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("LOAF_VALIDATE_TYPESCRIPT", "1")

	warnings, err := validateNativeBuildArtifacts(root, "opencode")
	if err != nil {
		t.Fatalf("validateNativeBuildArtifacts error = %v, want fake tsc success", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none when tsc is present", warnings)
	}
	log := readBuildFileString(t, logPath)
	for _, want := range []string{"--noEmit", "--allowJs false", filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts")} {
		if !strings.Contains(log, want) {
			t.Fatalf("tsc log = %q, want %q", log, want)
		}
	}
}

func TestNativeBuildValidationRejectsMalformedTypeScriptWhenEnabled(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "dist", "opencode", "plugins"))
	writeFile(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"), "const broken: = true;\n")
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "tsc"), strings.Join([]string{
		"#!/bin/sh",
		"echo 'error TS1005: type expected.'",
		"exit 2",
		"",
	}, "\n"))
	if err := os.Chmod(filepath.Join(bin, "tsc"), 0o755); err != nil {
		t.Fatalf("Chmod(tsc) error = %v", err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("LOAF_VALIDATE_TYPESCRIPT", "1")

	warnings, err := validateNativeBuildArtifacts(root, "opencode")
	if err == nil {
		t.Fatal("validateNativeBuildArtifacts error = nil, want TypeScript validation failure")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none on TypeScript validation failure", warnings)
	}
	if !strings.Contains(err.Error(), "TypeScript validation failed") || !strings.Contains(err.Error(), "TS1005") {
		t.Fatalf("error = %v, want TypeScript diagnostic", err)
	}
}

func TestNoHarnessProseSubstitution(t *testing.T) {
	// Contract: no markdown transform on skill-copy, OpenCode command generation,
	// or agent-copy paths rewrites authored prose. Probe every target with the
	// pre-substitution forms so a reintroduced replacer would emit banned products.
	samples := harnessProseSubstitutionProbeSamples()
	probeBody := strings.Join(samples, "\n") + "\n"
	identity := func(content string) string { return content }
	if err := skillMarkdownTransformIsIdentity(identity, samples); err != nil {
		t.Fatal(err)
	}
	if err := skillMarkdownTransformIsIdentity(nil, samples); err == nil {
		t.Fatal("skillMarkdownTransformIsIdentity(nil) = nil, want non-nil (nil transform must not vacuously pass)")
	}

	root := setupBuildCommandLoafRoot(t)
	seedNativeBuildParityFixture(t, root)
	mkdirAll(t, filepath.Join(root, "content", "skills", "demo", "templates"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "references", "probe.md"), probeBody)
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "templates", "probe.md"), probeBody)
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.claude-code.yaml"), strings.Join([]string{
		"user-invocable: true",
		"allowed-tools: Bash",
		"",
	}, "\n"))
	// Overwrite the skill body so SKILL.md itself carries the probe samples.
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.md"), strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo skill that has enough words to require folded YAML output from gray matter when the native builder writes frontmatter for generated skills.",
		"---",
		"",
		"# Demo",
		"",
		probeBody,
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.md"), strings.Join([]string{
		"---",
		"name: implementer",
		"description: Implementer probe agent.",
		"---",
		"",
		"# Implementer",
		"",
		probeBody,
	}, "\n"))
	// Claude agents require sidecars; keep a minimal one so the full build succeeds.
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.claude-code.yaml"), strings.Join([]string{
		"name: implementer",
		"description: Implementer probe agent.",
		"",
	}, "\n"))

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}

	banned := harnessProseSubstitutionBannedProducts()
	checkProbe := func(label, body string) {
		t.Helper()
		for _, sample := range samples {
			if !strings.Contains(body, sample) {
				t.Errorf("%s missing probe sample %q", label, sample)
			}
		}
		for _, phrase := range banned {
			if strings.Contains(body, phrase) {
				t.Errorf("%s contains harness-substituted prose %q", label, phrase)
			}
		}
	}

	for _, target := range defaultBuildTargets {
		skillsDir := nativeBuildSkillTreeDir(root, target)
		checkProbe(target+" skills/demo/SKILL.md", readBuildFileString(t, filepath.Join(skillsDir, "demo", "SKILL.md")))
		checkProbe(target+" skills/demo/references/probe.md", readBuildFileString(t, filepath.Join(skillsDir, "demo", "references", "probe.md")))
		checkProbe(target+" skills/demo/templates/probe.md", readBuildFileString(t, filepath.Join(skillsDir, "demo", "templates", "probe.md")))
	}

	opencodeCommand := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "commands", "demo.md"))
	checkProbe("opencode commands/demo.md", opencodeCommand)

	for _, target := range []string{"claude-code", "opencode", "cursor"} {
		agentPath := filepath.Join(nativeBuildTargetOutputDir(root, target), "agents", "implementer.md")
		checkProbe(target+" agents/implementer.md", readBuildFileString(t, agentPath))
	}
}

func TestSkillTreeIsTargetInvariant(t *testing.T) {
	root := setupIsolatedRepositoryBuildRoot(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}
	if err := compareNativeBuildSkillTrees(root, defaultBuildTargets); err != nil {
		t.Fatal(err)
	}
}

func TestLabeledHarnessSectionsRenderVerbatim(t *testing.T) {
	// Contract: labeled harness sections are authored content that ships
	// byte-identical on every target. Product tokens must appear inside their
	// own product section — never swapped across section boundaries, never
	// doubled by residual substitution.
	root := setupIsolatedRepositoryBuildRoot(t)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}

	type sectionExpect struct {
		heading string
		tokens  map[string]int // token -> exact count inside this section
	}
	type labeledFile struct {
		rel      string
		sections []sectionExpect
		// documentTokens are asserted on the full file (headings, shared prose).
		documentTokens map[string]int
	}
	files := []labeledFile{
		{
			rel: "foundations/references/permissions.md",
			documentTokens: map[string]int{
				"## Orchestrator Allowlists by Harness": 1,
				"## Permission Commands by Harness":     1,
				"### Codex":                             1,
				"### Backend Developer (Claude Code)":   1,
				"### Frontend Developer (Claude Code)":  1,
				"### DBA Agent (Claude Code)":           1,
				"### DevOps Agent (Claude Code)":        1,
			},
			sections: []sectionExpect{
				{
					heading: "### Codex",
					tokens: map[string]int{
						"update_plan":                      1,
						"TodoWrite, TodoRead":              0,
						"TodoWrite":                        0,
						"TodoRead":                         0,
						"Linear MCP tools (if configured)": 1,
						"exec_command":                     1, // warned against in prose; must not be an allowlist line
						"exec_command  #":                  0,
					},
				},
				{
					heading: "### Backend Developer (Claude Code)",
					tokens: map[string]int{
						"Bash(pytest *)": 1,
						"Bash(docker *)": 0,
					},
				},
				{
					heading: "### Frontend Developer (Claude Code)",
					tokens: map[string]int{
						"Bash(npm *)":    1,
						"Bash(docker *)": 0,
					},
				},
				{
					heading: "### DBA Agent (Claude Code)",
					tokens: map[string]int{
						"Bash(psql --help)": 1,
						"Write, Edit":       0,
					},
				},
				{
					heading: "### DevOps Agent (Claude Code)",
					tokens: map[string]int{
						"Bash(docker *)":                0,
						"Bash(docker ps *)":             1,
						"Bash(docker images *)":         1,
						"Bash(docker inspect *)":        1,
						"Bash(kubectl get *)":           0, // --profile-output write; approval-gated
						"Bash(terraform validate *)":    1,
						"Bash(terraform plan *)":        0,
						"Bash(docker compose config *)": 0,
						"--profile-output":              1,
					},
				},
			},
		},
		{
			rel: "orchestration/references/background-agents.md",
			documentTokens: map[string]int{
				"### Claude Code":     1,
				"### Cursor":          1,
				"### Amp":             1,
				"### Other harnesses": 1,
			},
			sections: []sectionExpect{
				{
					heading: "### Claude Code",
					tokens: map[string]int{
						`subagent_type="background-runner"`: 1,
						"run_in_background=True":            1,
						"is_background: true":               0,
						"delegate_grok_implementation":      0,
					},
				},
				{
					heading: "### Cursor",
					tokens: map[string]int{
						"is_background: true":               1,
						"@background-runner":                1,
						`subagent_type="background-runner"`: 0,
						"delegate_grok_implementation":      0,
					},
				},
				{
					heading: "### Amp",
					tokens: map[string]int{
						"delegate_grok_implementation": 1,
						"delegate_luna_review":         1,
						"loaf-medium":                  1,
						"loaf-ultra":                   1,
						"is_background: true":          0,
					},
				},
				{
					heading: "### Other harnesses",
					tokens: map[string]int{
						"@background-runner":                0,
						`subagent_type="background-runner"`: 0,
						"is_background: true":               0,
						"delegate_grok_implementation":      0,
					},
				},
			},
		},
	}
	// Historical substitution doubled the same replacement into adjacent
	// tokens (TodoWrite + TodoRead → update_plan, update_plan).
	banned := []string{
		"update_plan, update_plan",
		"TodoWrite, TodoWrite",
		"TodoRead, TodoRead",
		"native task/todo surface when available, native task/todo surface when available",
		"task list or chat checklist, task list or chat checklist",
		"Amp thread checklist, Amp thread checklist",
	}

	// permissions.md has multiple ### Claude Code headings (Permission Commands,
	// Recommended Allowlists, Orchestrator). Bind orchestrator tokens by
	// locating the Orchestrator parent, then its Claude Code child.
	var baseline map[string]string
	for _, target := range defaultBuildTargets {
		skillsDir := nativeBuildSkillTreeDir(root, target)
		for _, file := range files {
			path := filepath.Join(skillsDir, filepath.FromSlash(file.rel))
			body := readBuildFileString(t, path)
			for token, want := range file.documentTokens {
				if got := strings.Count(body, token); got != want {
					t.Errorf("%s skills/%s: %q appears %d times, want %d", target, file.rel, token, got, want)
				}
			}
			for _, section := range file.sections {
				sectionBody, ok := labeledHarnessSectionBody(body, section.heading)
				if !ok {
					t.Errorf("%s skills/%s missing section %q", target, file.rel, section.heading)
					continue
				}
				// For permissions orchestrator Claude fence, disambiguate among
				// multiple ### Claude Code headings by preferring the one under
				// the Orchestrator parent that contains TodoWrite.
				if file.rel == "foundations/references/permissions.md" && section.heading == "### Claude Code" {
					parent, ok := labeledHarnessSectionBody(body, "## Orchestrator Allowlists by Harness")
					if !ok {
						t.Errorf("%s skills/%s missing orchestrator parent", target, file.rel)
						continue
					}
					sectionBody, ok = labeledHarnessSectionBody(parent, "### Claude Code")
					if !ok {
						t.Errorf("%s skills/%s missing Claude Code under orchestrator parent", target, file.rel)
						continue
					}
				}
				if file.rel == "foundations/references/permissions.md" && section.heading == "### Codex" {
					parent, ok := labeledHarnessSectionBody(body, "## Orchestrator Allowlists by Harness")
					if !ok {
						t.Errorf("%s skills/%s missing orchestrator parent", target, file.rel)
						continue
					}
					sectionBody, ok = labeledHarnessSectionBody(parent, "### Codex")
					if !ok {
						t.Errorf("%s skills/%s missing Codex under orchestrator parent", target, file.rel)
						continue
					}
				}
				for token, want := range section.tokens {
					if got := strings.Count(sectionBody, token); got != want {
						t.Errorf("%s skills/%s section %q: %q appears %d times, want %d", target, file.rel, section.heading, token, got, want)
					}
				}
			}
			// Explicit cross-section binding for permissions orchestrator fences.
			if file.rel == "foundations/references/permissions.md" {
				parent, ok := labeledHarnessSectionBody(body, "## Orchestrator Allowlists by Harness")
				if !ok {
					t.Errorf("%s skills/%s missing orchestrator parent", target, file.rel)
				} else {
					claudeBody, ok := labeledHarnessSectionBody(parent, "### Claude Code")
					if !ok {
						t.Errorf("%s skills/%s missing Claude Code orchestrator section", target, file.rel)
					} else {
						for token, want := range map[string]int{
							"TodoWrite, TodoRead": 1,
							"update_plan":         0,
							"Read, Glob, Grep":    1,
							"Bash(date *)":        1,
							"Bash(git status)":    1,
						} {
							if got := strings.Count(claudeBody, token); got != want {
								t.Errorf("%s skills/%s Claude orchestrator: %q appears %d times, want %d", target, file.rel, token, got, want)
							}
						}
					}
					codexBody, ok := labeledHarnessSectionBody(parent, "### Codex")
					if !ok {
						t.Errorf("%s skills/%s missing Codex orchestrator section", target, file.rel)
					} else {
						for token, want := range map[string]int{
							"update_plan":                      1,
							"TodoWrite, TodoRead":              0,
							"TodoWrite":                        0,
							"exec_command":                     1,
							"exec_command  #":                  0,
							"Linear MCP tools (if configured)": 1,
							"prefix_rule":                      2,
							"~/.codex/rules/":                  1,
							"argument-scoped execution":        0,
						} {
							if got := strings.Count(codexBody, token); got != want {
								t.Errorf("%s skills/%s Codex orchestrator: %q appears %d times, want %d", target, file.rel, token, got, want)
							}
						}
					}
				}
			}
			for _, phrase := range banned {
				if strings.Contains(body, phrase) {
					t.Errorf("%s skills/%s contains doubled tool phrase %q", target, file.rel, phrase)
				}
			}
			if baseline == nil {
				baseline = map[string]string{}
			}
			if prev, ok := baseline[file.rel]; ok {
				if body != prev {
					t.Errorf("skills/%s differs between targets (not target-invariant under labeled sections)", file.rel)
				}
			} else {
				baseline[file.rel] = body
			}
		}
	}
}

func TestLabeledHarnessSectionBodyRejectsSubstringAndFencedHeading(t *testing.T) {
	doc := strings.Join([]string{
		"## Parent",
		"",
		"### Codex-extra",
		"",
		"wrong body",
		"",
		"```",
		"### Codex",
		"fenced body",
		"```",
		"",
		"### Codex",
		"",
		"real body",
		"",
		"### Other",
		"",
	}, "\n")
	body, ok := labeledHarnessSectionBody(doc, "### Codex")
	if !ok {
		t.Fatal("labeledHarnessSectionBody(### Codex) = false, want the real heading")
	}
	if strings.Contains(body, "wrong body") || strings.Contains(body, "fenced body") {
		t.Fatalf("body = %q, matched substring or fenced heading", body)
	}
	if !strings.Contains(body, "real body") {
		t.Fatalf("body = %q, want real body", body)
	}
	if _, ok := labeledHarnessSectionBody(doc, "### Codex-extra"); !ok {
		t.Fatal("labeledHarnessSectionBody(### Codex-extra) = false, want its own heading")
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectExecutableArtifactToken(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts")
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, "export const cmd = \"{{STRAY_TOKEN}} journal\";\n")

	err := validateNativeBuildUnresolvedPlaceholders(root, "amp")
	if err == nil || !strings.Contains(err.Error(), "{{STRAY_TOKEN}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want executable-artifact rejection", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectDollarPrefixedArbitraryToken(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "opencode", "plugins.ts")
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, "export const leak = \"${{ARBITRARY}}\";\n")

	err := validateNativeBuildUnresolvedPlaceholders(root, "opencode")
	if err == nil || !strings.Contains(err.Error(), "{{ARBITRARY}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want ${{ARBITRARY}} rejection", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectsNestedManifestBasename(t *testing.T) {
	root := realpath(t, t.TempDir())
	nested := filepath.Join(root, "dist", "codex", "nested", targetBuildManifestFile)
	mkdirAll(t, filepath.Dir(nested))
	writeFile(t, nested, "{\n  \"token\": \"{{NESTED_MANIFEST}}\"\n}\n")

	err := validateNativeBuildUnresolvedPlaceholders(root, "codex")
	if err == nil || !strings.Contains(err.Error(), "{{NESTED_MANIFEST}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want nested-manifest basename rejection", err)
	}
}

func TestNativeBuildSkillInvarianceRejectsForeignTargetSidecarKey(t *testing.T) {
	root := realpath(t, t.TempDir())
	sidecar := filepath.Join(root, "content", "skills", "demo", "SKILL.opencode.yaml")
	mkdirAll(t, filepath.Dir(sidecar))
	writeFile(t, sidecar, "allowed-tools: Bash(rm -rf *)\n")

	_, err := nativeBuildSkillSidecarAuthorizedValues(root, "demo", "opencode")
	if err == nil || !strings.Contains(err.Error(), "allowed-tools") || !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("error = %v, want foreign allowed-tools rejection for opencode sidecar", err)
	}
}

func TestNativeBuildSkillSidecarAuthorizedValuesRefusesNonRegularFile(t *testing.T) {
	root := realpath(t, t.TempDir())
	dir := filepath.Join(root, "content", "skills", "demo")
	mkdirAll(t, dir)
	sidecar := filepath.Join(dir, "SKILL.claude-code.yaml")
	if err := os.Mkdir(sidecar, 0o755); err != nil {
		t.Fatalf("Mkdir(sidecar-as-dir) = %v", err)
	}
	_, err := nativeBuildSkillSidecarAuthorizedValues(root, "demo", "claude-code")
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("error = %v, want errNotRegularFile", err)
	}
}

func TestNativeBuildSkillSidecarAuthorizedValuesRefusesSymlink(t *testing.T) {
	root := realpath(t, t.TempDir())
	dir := filepath.Join(root, "content", "skills", "demo")
	mkdirAll(t, dir)
	outside := filepath.Join(t.TempDir(), "escape.yaml")
	writeFile(t, outside, "allowed-tools: Bash(rm -rf *)\n")
	sidecar := filepath.Join(dir, "SKILL.claude-code.yaml")
	if err := os.Symlink(outside, sidecar); err != nil {
		t.Fatalf("Symlink(sidecar) = %v", err)
	}
	_, err := nativeBuildSkillSidecarAuthorizedValues(root, "demo", "claude-code")
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("error = %v, want errNotRegularFile for symlinked sidecar", err)
	}
}

func TestReadNativeBuildAgentSidecarRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escape.yaml")
	writeFile(t, outside, "description: escaped\n")
	sidecar := filepath.Join(dir, "reviewer.cursor.yaml")
	if err := os.Symlink(outside, sidecar); err != nil {
		t.Fatalf("Symlink(sidecar) = %v", err)
	}
	_, err := readNativeBuildAgentSidecar(sidecar, false)
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("error = %v, want errNotRegularFile for symlinked agent sidecar", err)
	}
}

func TestCopyNativeBuildAgentsRefusesSymlinkedCursorSidecar(t *testing.T) {
	root := realpath(t, t.TempDir())
	agents := filepath.Join(root, "content", "agents")
	mkdirAll(t, agents)
	writeFile(t, filepath.Join(agents, "reviewer.md"), "# Reviewer\n\nRead-only audits.\n")
	outside := filepath.Join(t.TempDir(), "escape.yaml")
	writeFile(t, outside, "name: reviewer\ndescription: escaped\n")
	if err := os.Symlink(outside, filepath.Join(agents, "reviewer.cursor.yaml")); err != nil {
		t.Fatalf("Symlink(cursor sidecar) = %v", err)
	}
	err := copyNativeBuildAgents(filepath.Join(root, "content"), filepath.Join(root, "dist", "cursor", "agents"), "cursor", "1.0.0", nil, false)
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("copyNativeBuildAgents error = %v, want errNotRegularFile via Cursor sidecar path", err)
	}
}

func TestCopyNativeBuildAgentsRefusesSymlinkedAgentBody(t *testing.T) {
	root := realpath(t, t.TempDir())
	agents := filepath.Join(root, "content", "agents")
	mkdirAll(t, agents)
	outside := filepath.Join(t.TempDir(), "escape.md")
	writeFile(t, outside, "# Escaped\n")
	if err := os.Symlink(outside, filepath.Join(agents, "reviewer.md")); err != nil {
		t.Fatalf("Symlink(agent body) = %v", err)
	}
	err := copyNativeBuildAgents(filepath.Join(root, "content"), filepath.Join(root, "dist", "cursor", "agents"), "cursor", "1.0.0", nil, false)
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("copyNativeBuildAgents error = %v, want errNotRegularFile for symlinked agent body", err)
	}
}

func TestGenerateNativeOpenCodeCommandsRefusesSymlinkedOpencodeSidecar(t *testing.T) {
	root := realpath(t, t.TempDir())
	skill := filepath.Join(root, "content", "skills", "demo")
	mkdirAll(t, skill)
	writeFile(t, filepath.Join(skill, "SKILL.claude-code.yaml"), "user-invocable: true\n")
	outside := filepath.Join(t.TempDir(), "escape.yaml")
	writeFile(t, outside, "description: escaped\n")
	if err := os.Symlink(outside, filepath.Join(skill, "SKILL.opencode.yaml")); err != nil {
		t.Fatalf("Symlink(opencode sidecar) = %v", err)
	}
	built := filepath.Join(root, "dist", "skills", "demo")
	mkdirAll(t, built)
	writeFile(t, filepath.Join(built, "SKILL.md"), "---\nname: demo\ndescription: Demo\n---\n\n# Demo\n")
	err := generateNativeOpenCodeCommands(root, "1.0.0")
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("generateNativeOpenCodeCommands error = %v, want errNotRegularFile via OpenCode sidecar path", err)
	}
}

func TestGenerateNativeOpenCodeCommandsRefusesSymlinkedClaudeSidecar(t *testing.T) {
	root := realpath(t, t.TempDir())
	skill := filepath.Join(root, "content", "skills", "demo")
	mkdirAll(t, skill)
	outside := filepath.Join(t.TempDir(), "escape.yaml")
	writeFile(t, outside, "user-invocable: true\n")
	if err := os.Symlink(outside, filepath.Join(skill, "SKILL.claude-code.yaml")); err != nil {
		t.Fatalf("Symlink(claude-code sidecar) = %v", err)
	}
	built := filepath.Join(root, "dist", "skills", "demo")
	mkdirAll(t, built)
	writeFile(t, filepath.Join(built, "SKILL.md"), "---\nname: demo\ndescription: Demo\n---\n\n# Demo\n")
	err := generateNativeOpenCodeCommands(root, "1.0.0")
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("generateNativeOpenCodeCommands error = %v, want errNotRegularFile via Claude sidecar invocable check", err)
	}
}

func TestWriteNativeBuildSkillMarkdownRefusesSymlinkedSkillBody(t *testing.T) {
	root := realpath(t, t.TempDir())
	skillSrc := filepath.Join(root, "dist", "skills", "demo")
	mkdirAll(t, skillSrc)
	outside := filepath.Join(t.TempDir(), "escape.md")
	writeFile(t, outside, "---\nname: demo\ndescription: escaped\n---\n\n# Escaped\n")
	if err := os.Symlink(outside, filepath.Join(skillSrc, "SKILL.md")); err != nil {
		t.Fatalf("Symlink(SKILL.md) = %v", err)
	}
	err := writeNativeBuildSkillMarkdown(skillSrc, filepath.Join(root, "dist", "cursor", "skills", "demo"), nativeBuildSkillCopyOptions{
		targetName: "cursor",
		version:    "1.0.0",
	})
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("writeNativeBuildSkillMarkdown error = %v, want errNotRegularFile for symlinked skill body", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectExtraCodexToken(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "codex", ".codex", "hooks.json")
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, "{\n  \"hooks\": {\n    \"SessionStart\": [{\n      \"matcher\": \"startup|resume|clear|compact\",\n      \"hooks\": [{\n        \"type\": \"command\",\n        \"command\": \"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook {{OTHER}}\"\n      }]\n    }]\n  }\n}\n")

	err := validateNativeBuildUnresolvedPlaceholders(root, "codex")
	if err == nil || !strings.Contains(err.Error(), "unresolved harness token") || !strings.Contains(err.Error(), "{{OTHER}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want extra unresolved token rejection", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersAllowsInstallTimeCodexTokens(t *testing.T) {
	root := realpath(t, t.TempDir())
	hooks := filepath.Join(root, "dist", "codex", ".codex", "hooks.json")
	rules := filepath.Join(root, "dist", "codex", ".codex", "rules", "loaf.rules.tmpl")
	mkdirAll(t, filepath.Dir(hooks))
	mkdirAll(t, filepath.Dir(rules))
	writeFile(t, hooks, "{\n  \"hooks\": {\n    \"SessionStart\": [{\n      \"matcher\": \"startup|resume|clear|compact\",\n      \"hooks\": [{\n        \"type\": \"command\",\n        \"command\": \"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook\",\n        \"commandWindows\": \"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook\"\n      }]\n    }]\n  }\n}\n")
	writeFile(t, rules, "# Loaf Codex policy\n{{LOAF_BASIC_RULES}}\n")

	if err := validateNativeBuildUnresolvedPlaceholders(root, "codex"); err != nil {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want allow install-time tokens", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectsTextBinArtifactToken(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "plugins", "loaf", "bin", "loaf")
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, "#!/usr/bin/env node\nconsole.log('{{STRAY_BIN_TOKEN}}');\n")

	err := validateNativeBuildUnresolvedPlaceholders(root, "claude-code")
	if err == nil || !strings.Contains(err.Error(), "{{STRAY_BIN_TOKEN}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want text bin/ token rejection", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersSkipsOpaqueBinaryByMagic(t *testing.T) {
	root := realpath(t, t.TempDir())
	// Outside bin/: the pre-fix wholesale bin/ skip would not cover this path,
	// so a green result proves magic classification rather than directory exclusion.
	path := filepath.Join(root, "plugins", "loaf", ".claude", "hooks", "session-start")
	mkdirAll(t, filepath.Dir(path))
	// Genuine Mach-O 64-bit little-endian magic (darwin-arm64 loaf header), then a
	// {{ span with no NUL — the old "contains NUL" heuristic would have scanned
	// this and false-rejected.
	payload := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("rest-of-header{{FAKE_TOKEN}}tail")...)
	if err := os.WriteFile(path, payload, 0o755); err != nil {
		t.Fatalf("WriteFile(mach-o fixture) = %v", err)
	}

	if err := validateNativeBuildUnresolvedPlaceholders(root, "claude-code"); err != nil {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want opaque Mach-O skipped", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRejectsNULBearingTextToken(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "plugins", "loaf", ".claude", "hooks", "nul-text.sh")
	mkdirAll(t, filepath.Dir(path))
	// A text artifact with an embedded NUL must not evade the scan: opacity is
	// magic-based, not "any NUL anywhere".
	if err := os.WriteFile(path, []byte("prefix\x00{{STRAY_NUL_TOKEN}}suffix\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(nul text) = %v", err)
	}

	err := validateNativeBuildUnresolvedPlaceholders(root, "claude-code")
	if err == nil || !strings.Contains(err.Error(), "{{STRAY_NUL_TOKEN}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want NUL-bearing text token rejection", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersSkipsELFBinaryWithoutNUL(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "amp", ".amp", "plugins", "helper")
	mkdirAll(t, filepath.Dir(path))
	payload := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("no-nul-body{{ELF_TOKEN}}")...)
	if err := os.WriteFile(path, payload, 0o755); err != nil {
		t.Fatalf("WriteFile(elf fixture) = %v", err)
	}

	if err := validateNativeBuildUnresolvedPlaceholders(root, "amp"); err != nil {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want no-NUL ELF skipped", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersSkipsPEBinaryWithoutNUL(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "codex", ".codex", "helper.exe")
	mkdirAll(t, filepath.Dir(path))
	payload := append([]byte{'M', 'Z'}, []byte("pe-stub{{PE_TOKEN}}")...)
	if err := os.WriteFile(path, payload, 0o755); err != nil {
		t.Fatalf("WriteFile(pe fixture) = %v", err)
	}

	if err := validateNativeBuildUnresolvedPlaceholders(root, "codex"); err != nil {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want no-NUL PE skipped", err)
	}
}

func TestIsOpaqueNativeBuildArtifactMagic(t *testing.T) {
	if !isOpaqueNativeBuildArtifact([]byte{0xcf, 0xfa, 0xed, 0xfe, 0x00}) {
		t.Fatal("Mach-O 64 LE should be opaque")
	}
	if !isOpaqueNativeBuildArtifact([]byte{0x7f, 'E', 'L', 'F'}) {
		t.Fatal("ELF should be opaque")
	}
	if !isOpaqueNativeBuildArtifact([]byte{'M', 'Z', 'x'}) {
		t.Fatal("PE/MZ should be opaque")
	}
	if isOpaqueNativeBuildArtifact([]byte("\x00{{TOKEN}}")) {
		t.Fatal("NUL-bearing text must not be opaque")
	}
	if isOpaqueNativeBuildArtifact([]byte("plain{{TOKEN}}")) {
		t.Fatal("plain text must not be opaque")
	}
}

func TestNativeBuildUnresolvedPlaceholdersFindsTokenPastProbeInUnknownMagic(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "amp", ".amp", "plugins", "odd.bin")
	mkdirAll(t, filepath.Dir(path))
	// Unknown magic (not Mach-O/ELF/PE): must still be scanned, not skipped, and
	// must not require loading the whole artifact before the token line.
	payload := make([]byte, nativeBuildOpacityProbeBytes+64)
	payload[0], payload[1], payload[2], payload[3] = 0x01, 0x02, 0x03, 0x04
	copy(payload[nativeBuildOpacityProbeBytes:], []byte("tail{{UNKNOWN_MAGIC_TOKEN}}\n"))
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(unknown magic) = %v", err)
	}
	err := validateNativeBuildUnresolvedPlaceholders(root, "amp")
	if err == nil || !strings.Contains(err.Error(), "{{UNKNOWN_MAGIC_TOKEN}}") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want token past probe on unknown magic", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersRefusesOversizedLineUnknownMagic(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "codex", ".codex", "blob.bin")
	mkdirAll(t, filepath.Dir(path))
	// No newlines and larger than the scanner's max token: refuse with a bounded
	// error rather than ReadAll into memory. Unknown magic must not be skipped.
	payload := make([]byte, projectFileReadLimit+1024)
	payload[0], payload[1], payload[2], payload[3] = 0xde, 0xad, 0xbe, 0xef
	for i := 4; i < len(payload); i++ {
		payload[i] = 'x'
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(oversized line) = %v", err)
	}
	err := validateNativeBuildUnresolvedPlaceholders(root, "codex")
	if err == nil || !strings.Contains(err.Error(), "refusing unbounded read") {
		t.Fatalf("validateNativeBuildUnresolvedPlaceholders error = %v, want bounded refusal for oversized unknown-magic line", err)
	}
}

func TestNativeBuildUnresolvedPlaceholdersCapsFindingsWithSuppressionCount(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, "dist", "amp", ".amp", "plugins", "tokens.ts")
	mkdirAll(t, filepath.Dir(path))
	total := nativeBuildUnresolvedPlaceholderFindingCap + 8
	var body strings.Builder
	for i := 1; i <= total; i++ {
		body.WriteString(fmt.Sprintf("{{TOKEN_%d}}\n", i))
	}
	writeFile(t, path, body.String())

	err := validateNativeBuildUnresolvedPlaceholders(root, "amp")
	if err == nil {
		t.Fatal("validateNativeBuildUnresolvedPlaceholders error = nil, want capped findings")
	}
	msg := err.Error()
	wantShown := fmt.Sprintf("showing %d of %d findings (%d suppressed)",
		nativeBuildUnresolvedPlaceholderFindingCap, total, total-nativeBuildUnresolvedPlaceholderFindingCap)
	if !strings.Contains(msg, wantShown) {
		t.Fatalf("error = %q, want %q", msg, wantShown)
	}
	if !strings.Contains(msg, "{{TOKEN_1}}") {
		t.Fatalf("error = %q, want first retained token", msg)
	}
	if strings.Contains(msg, fmt.Sprintf("{{TOKEN_%d}}", total)) {
		t.Fatalf("error = %q, must not list suppressed token {{TOKEN_%d}}", msg, total)
	}
}

func TestConfirmOpenedRegularFileNoFollowRejectsLeafSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.txt")
	writeFile(t, target, "escaped-contents\n")
	leaf := filepath.Join(dir, "sidecar.yaml")
	writeFile(t, leaf, "original\n")
	before, err := os.Lstat(leaf)
	if err != nil {
		t.Fatalf("Lstat(before) = %v", err)
	}
	// Simulate the portable race: after Lstat, replace the leaf with a symlink.
	if err := os.Remove(leaf); err != nil {
		t.Fatalf("Remove(leaf) = %v", err)
	}
	if err := os.Symlink(target, leaf); err != nil {
		t.Fatalf("Symlink(leaf) = %v", err)
	}
	file, err := os.Open(leaf) // follows, as the portable Open would
	if err != nil {
		t.Fatalf("Open(swapped leaf) = %v", err)
	}
	defer file.Close()
	err = confirmOpenedRegularFileNoFollow(leaf, file, before)
	if err == nil || !errors.Is(err, errNotRegularFile) {
		t.Fatalf("confirmOpenedRegularFileNoFollow = %v, want errNotRegularFile after leaf symlink swap", err)
	}
}

func TestConfirmOpenedRegularFileNoFollowAcceptsStableRegularFile(t *testing.T) {
	dir := t.TempDir()
	leaf := filepath.Join(dir, "sidecar.yaml")
	writeFile(t, leaf, "stable\n")
	before, err := os.Lstat(leaf)
	if err != nil {
		t.Fatalf("Lstat = %v", err)
	}
	file, err := os.Open(leaf)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer file.Close()
	if err := confirmOpenedRegularFileNoFollow(leaf, file, before); err != nil {
		t.Fatalf("confirmOpenedRegularFileNoFollow(stable) = %v, want nil", err)
	}
}

func TestNativeBuildSkillInvarianceRejectsUnauthorizedSidecarField(t *testing.T) {
	// A sidecar-owned key whose value did not come from the sidecar must fail,
	// not be silently stripped.
	_, _, err := normalizeNativeBuildSkillFileForInvariance("demo/SKILL.md", strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo",
		"allowed-tools: Bash(rm -rf *)",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
	}, "\n"), map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "allowed-tools") {
		t.Fatalf("error = %v, want unauthorized allowed-tools rejection", err)
	}
}

func TestNativeBuildSkillInvarianceSeesNestedFrontmatter(t *testing.T) {
	strict := strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo",
		"metadata:",
		"  mode: strict",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
	}, "\n")
	permissive := strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo",
		"metadata:",
		"  mode: permissive",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
	}, "\n")
	gotStrict, _, err := normalizeNativeBuildSkillFileForInvariance("demo/SKILL.md", strict, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotPermissive, _, err := normalizeNativeBuildSkillFileForInvariance("demo/SKILL.md", permissive, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotStrict == gotPermissive {
		t.Fatalf("nested frontmatter compared equal after normalize:\n strict: %q\n permissive: %q", gotStrict, gotPermissive)
	}
	if !strings.Contains(gotStrict, "mode: strict") || !strings.Contains(gotPermissive, "mode: permissive") {
		t.Fatalf("normalize dropped nested frontmatter:\n strict: %q\n permissive: %q", gotStrict, gotPermissive)
	}

	listA := strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo",
		"tags:",
		"  - alpha",
		"  - beta",
		"# retained comment between keys",
		"metadata:",
		"  nested:",
		"    leaf: one",
		"notes: |",
		"  line one",
		"  line two",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
	}, "\n")
	listB := strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo",
		"tags:",
		"  - alpha",
		"  - gamma",
		"# retained comment between keys",
		"metadata:",
		"  nested:",
		"    leaf: two",
		"notes: |",
		"  line one",
		"  line two changed",
		"version: 1.0.0",
		"---",
		"",
		"# Demo",
		"",
	}, "\n")
	gotA, _, err := normalizeNativeBuildSkillFileForInvariance("demo/SKILL.md", listA, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotB, _, err := normalizeNativeBuildSkillFileForInvariance("demo/SKILL.md", listB, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotA == gotB {
		t.Fatalf("list/deeper/block frontmatter compared equal after normalize:\n a: %q\n b: %q", gotA, gotB)
	}
	for _, want := range []string{"- beta", "leaf: one", "line two", "# retained comment between keys"} {
		if !strings.Contains(gotA, want) {
			t.Fatalf("normalize dropped %q from complex frontmatter:\n %q", want, gotA)
		}
	}
	for _, want := range []string{"- gamma", "leaf: two", "line two changed"} {
		if !strings.Contains(gotB, want) {
			t.Fatalf("normalize dropped %q from complex frontmatter:\n %q", want, gotB)
		}
	}
}

func TestNativeBuildParityMatrixDerivesFromSource(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeBuildParityFixture(t, root)
	var stdout bytes.Buffer

	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}
	expectations, err := nativeBuildParityExpectationsFromSource(root)
	if err != nil {
		t.Fatalf("nativeBuildParityExpectationsFromSource error = %v", err)
	}
	if err := assertNativeBuildParityReachability(root, expectations); err != nil {
		t.Fatalf("assertNativeBuildParityReachability error = %v", err)
	}
	if err := assertNativeBuildParityHookSemantics(root, expectations); err != nil {
		t.Fatalf("assertNativeBuildParityHookSemantics error = %v", err)
	}
}

func TestNativeBuildParityMatrixDetectsSeededReachabilityGap(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeBuildParityFixture(t, root)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}
	expectations, err := nativeBuildParityExpectationsFromSource(root)
	if err != nil {
		t.Fatalf("nativeBuildParityExpectationsFromSource error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, "dist", "opencode", "commands", "workflow-only.md")); err != nil {
		t.Fatalf("Remove(workflow-only command) error = %v", err)
	}

	err = assertNativeBuildParityReachability(root, expectations)
	if err == nil || !strings.Contains(err.Error(), "opencode command workflow-only") {
		t.Fatalf("assertNativeBuildParityReachability error = %v, want seeded opencode command gap", err)
	}
}

func TestNativeBuildParityMatrixDetectsSeededHookSemanticGap(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeBuildParityFixture(t, root)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build"}); err != nil {
		t.Fatalf("build error = %v\n%s", err, stdout.String())
	}
	expectations, err := nativeBuildParityExpectationsFromSource(root)
	if err != nil {
		t.Fatalf("nativeBuildParityExpectationsFromSource error = %v", err)
	}
	path := filepath.Join(root, "dist", "codex", ".codex", "hooks.json")
	body := readBuildFileString(t, path)
	body = `{"hooks":{"SessionStart":[]}}`
	writeFile(t, path, body)

	err = assertNativeBuildParityHookSemantics(root, expectations)
	if err == nil || !strings.Contains(err.Error(), "codex hook check-secrets missing SessionStart context group") {
		t.Fatalf("assertNativeBuildParityHookSemantics error = %v, want seeded hook semantic gap", err)
	}
}

// setupIsolatedRepositoryBuildRoot points a temporary loaf root at the
// repository's content/, config/, and bin/ via symlinks and a copied
// package.json, so tests can run `loaf build` without rewriting the real
// plugins/ or dist/ trees (and without deleting untracked files there).
func setupIsolatedRepositoryBuildRoot(t *testing.T) string {
	t.Helper()
	repo := testRepositoryRoot(t)
	if _, err := os.Stat(filepath.Join(repo, "content", "skills")); err != nil {
		t.Skipf("repository content/skills unavailable: %v", err)
	}
	root := realpath(t, t.TempDir())
	for _, name := range []string{"content", "config", "bin"} {
		src := filepath.Join(repo, name)
		if _, err := os.Stat(src); err != nil {
			t.Fatalf("stat(%s) error = %v", src, err)
		}
		if err := os.Symlink(src, filepath.Join(root, name)); err != nil {
			t.Fatalf("Symlink(%s) error = %v", name, err)
		}
	}
	packageJSON, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	return root
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
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
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
		"  amp:",
		"    output: dist/amp/",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(targets.yaml) error = %v", err)
	}
	capabilities, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatalf("ReadFile(target-capabilities.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, TargetCapabilityEvidenceRecordPath), capabilities, 0o644); err != nil {
		t.Fatalf("WriteFile(target-capabilities.json) error = %v", err)
	}
	return root
}

func readBuildJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return result
}

func seedNativeCodexBuildFixture(t *testing.T, root string) {
	t.Helper()
	mkdirAll(t, filepath.Join(root, "content", "skills", "demo", "references"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "demo", "scripts"))
	mkdirAll(t, filepath.Join(root, "content", "templates"))
	mkdirAll(t, filepath.Join(root, "content", "codex", "rules"))
	writeFile(t, filepath.Join(root, "content", "codex", "rules", "loaf.rules.tmpl"), "# Loaf Codex policy\n{{LOAF_BASIC_RULES}}\n")
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
		"  amp:",
		"    output: dist/amp/",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "config", "hooks.yaml"), strings.Join([]string{
		"hooks:",
		"  pre-tool:",
		"    - id: check-secrets",
		"      matcher: \"Edit|Write|Bash\"",
		"      blocking: true",
		"      timeout: 30000",
		"      failClosed: true",
		"      description: Check for hardcoded secrets before writing",
		"    - id: security-audit",
		"      matcher: \"Bash\"",
		"      blocking: true",
		"      timeout: 600000",
		"      failClosed: true",
		"      description: Run security audit on bash commands",
		"    - id: ephemeral-provenance",
		"      matcher: \"Bash\"",
		"      if: \"Bash(git push:*)\"",
		"      blocking: true",
		"      timeout: 30000",
		"      failClosed: true",
		"      description: Block active specs from pointing at deleted ephemeral Markdown",
		"    - id: github-account",
		"      matcher: \"Bash\"",
		"      blocking: true",
		"      timeout: 5000",
		"      failClosed: true",
		"      description: Block gh commands when active GitHub account differs from project config",
		"    - id: validate-push",
		"      matcher: \"Bash\"",
		"      if: \"Bash(git push:*)\"",
		"      blocking: false",
		"      timeout: 60000",
		"      description: Validates version bump, CHANGELOG, and build before git push",
		"    - id: workflow-pre-pr",
		"      matcher: \"Bash\"",
		"      if: \"Bash(gh pr create:*)\"",
		"      blocking: false",
		"      timeout: 5000",
		"      description: Remind about PR format before gh pr create",
		"    - id: validate-commit",
		"      matcher: \"Bash\"",
		"      if: \"Bash(git commit:*)\"",
		"      blocking: true",
		"      timeout: 30000",
		"      failClosed: true",
		"      description: Validate commit messages follow conventions",
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
		"      command: 'loaf journal context --from-hook'",
		"      event: SessionStart",
		"      description: Emit the layered continuity digest at conversation start",
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
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.claude-code.yaml"), strings.Join([]string{
		"user-invocable: true",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.opencode.yaml"), strings.Join([]string{
		"subtask: false",
		"",
	}, "\n"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "workflow-only"))
	writeFile(t, filepath.Join(root, "content", "skills", "workflow-only", "SKILL.md"), strings.Join([]string{
		"---",
		"name: workflow-only",
		"description: Workflow-only skill without an OpenCode sidecar.",
		"---",
		"",
		"# Workflow Only",
		"",
		"Run {{IMPLEMENT_CMD}} from a command generated by reachability.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "workflow-only", "SKILL.claude-code.yaml"), strings.Join([]string{
		"user-invocable: true",
		"",
	}, "\n"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "reference-only"))
	writeFile(t, filepath.Join(root, "content", "skills", "reference-only", "SKILL.md"), strings.Join([]string{
		"---",
		"name: reference-only",
		"description: Reference-only skill.",
		"---",
		"",
		"# Reference Only",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "reference-only", "SKILL.claude-code.yaml"), strings.Join([]string{
		"user-invocable: false",
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

func seedNativeBuildParityFixture(t *testing.T, root string) {
	t.Helper()
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	seedNativeOpenCodeBuildFixture(t, root)
	seedNativeClaudeCodeBuildFixture(t, root)
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.claude-code.yaml"), strings.Join([]string{
		"user-invocable: true",
		"allowed-tools: Bash",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "workflow-only", "SKILL.md"), strings.Join([]string{
		"---",
		"name: workflow-only",
		"description: Workflow-only skill without target-specific command sidecars.",
		"---",
		"",
		"# Workflow Only",
		"",
		"Use AskUserQuestionTool, TodoWrite, CLAUDE.md, /loaf:implement, and subagent language.",
		"",
	}, "\n"))
}

type nativeBuildParityExpectations struct {
	targets        []string
	workflowSkills []string
	preToolHooks   []nativeBuildHook
}

func nativeBuildParityExpectationsFromSource(root string) (nativeBuildParityExpectations, error) {
	targets, err := nativeBuildTargetNames(root)
	if err != nil {
		return nativeBuildParityExpectations{}, err
	}
	if strings.Join(targets, ",") != strings.Join(defaultBuildTargets, ",") {
		return nativeBuildParityExpectations{}, fmt.Errorf("targets = %v, want exactly %v", targets, defaultBuildTargets)
	}
	workflowSkills, err := nativeBuildParityUserInvocableSkills(root)
	if err != nil {
		return nativeBuildParityExpectations{}, err
	}
	hooks, err := readNativeBuildHooks(filepath.Join(root, "config", "hooks.yaml"))
	if err != nil {
		return nativeBuildParityExpectations{}, err
	}
	var preToolHooks []nativeBuildHook
	for _, hook := range hooks {
		if hook.section == "pre-tool" && hook.typeName != "prompt" {
			preToolHooks = append(preToolHooks, hook)
		}
	}
	return nativeBuildParityExpectations{
		targets:        targets,
		workflowSkills: workflowSkills,
		preToolHooks:   preToolHooks,
	}, nil
}

func nativeBuildParityUserInvocableSkills(root string) ([]string, error) {
	skillsDir := filepath.Join(root, "content", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}
	var skills []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sidecarPath := filepath.Join(skillsDir, entry.Name(), "SKILL.claude-code.yaml")
		fields, err := readNativeBuildAgentSidecar(sidecarPath, false)
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			if field.key == "user-invocable" && field.value.kind == "bool" && field.value.scalar == "true" {
				skills = append(skills, entry.Name())
			}
		}
	}
	sort.Strings(skills)
	return skills, nil
}

func assertNativeBuildParityReachability(root string, expectations nativeBuildParityExpectations) error {
	if len(expectations.workflowSkills) == 0 {
		return fmt.Errorf("no source user-invocable workflow skills found")
	}
	for _, target := range expectations.targets {
		for _, skill := range expectations.workflowSkills {
			skillPath := filepath.Join(nativeBuildTargetOutputDir(root, target), "skills", skill, "SKILL.md")
			if _, err := os.Stat(skillPath); err != nil {
				return fmt.Errorf("%s skill %s not reachable at %s: %w", target, skill, nativeBuildRelativePath(root, skillPath), err)
			}
			if target == "opencode" {
				commandPath := filepath.Join(root, "dist", "opencode", "commands", skill+".md")
				if _, err := os.Stat(commandPath); err != nil {
					return fmt.Errorf("opencode command %s not reachable at %s: %w", skill, nativeBuildRelativePath(root, commandPath), err)
				}
			}
		}
	}
	return nil
}

func assertNativeBuildParityHookSemantics(root string, expectations nativeBuildParityExpectations) error {
	for _, hook := range expectations.preToolHooks {
		if err := assertNativeBuildClaudeHookSemantics(root, hook); err != nil {
			return err
		}
		if err := assertNativeBuildCursorHookSemantics(root, hook); err != nil {
			return err
		}
		if err := assertNativeBuildOpenCodeHookSemantics(root, hook); err != nil {
			return err
		}
		if err := assertNativeBuildAmpHookSemantics(root, hook); err != nil {
			return err
		}
		if hook.section == "pre-tool" && codexEnforcementHooks[hook.id] && strings.Contains(hook.matcher, "Bash") {
			if err := assertNativeBuildCodexHookSemantics(root, hook); err != nil {
				return err
			}
		}
	}
	return nil
}

func assertNativeBuildClaudeHookSemantics(root string, hook nativeBuildHook) error {
	var payload struct {
		Hooks struct {
			PreToolUse []struct {
				Hooks []map[string]any `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	if err := readNativeBuildJSON(filepath.Join(root, "plugins", "loaf", "hooks", "hooks.json"), &payload); err != nil {
		return err
	}
	entry, ok := findNativeBuildGenericHook(payload.Hooks.PreToolUse, "command", nativeClaudeHookCommand(hook))
	if !ok {
		return fmt.Errorf("claude-code hook %s missing command %q", hook.id, nativeClaudeHookCommand(hook))
	}
	if got := nativeBuildGenericBool(entry, "failClosed"); got != hook.failClosed {
		return fmt.Errorf("claude-code hook %s failClosed = %v, want %v", hook.id, got, hook.failClosed)
	}
	return nil
}

func assertNativeBuildCursorHookSemantics(root string, hook nativeBuildHook) error {
	var payload nativeCursorHooksJSON
	if err := readNativeBuildJSON(filepath.Join(root, "dist", "cursor", "hooks.json"), &payload); err != nil {
		return err
	}
	command := nativeCursorHookCommand(hook)
	for _, entry := range payload.Hooks.PreToolUse {
		if entry.Command == command {
			if entry.FailClosed != hook.failClosed {
				return fmt.Errorf("cursor hook %s failClosed = %v, want %v", hook.id, entry.FailClosed, hook.failClosed)
			}
			if entry.If != hook.ifCondition {
				return fmt.Errorf("cursor hook %s if = %q, want %q", hook.id, entry.If, hook.ifCondition)
			}
			return nil
		}
	}
	return fmt.Errorf("cursor hook %s missing command %q", hook.id, command)
}

func assertNativeBuildCodexHookSemantics(root string, hook nativeBuildHook) error {
	var payload nativeCodexHooksJSON
	if err := readNativeBuildJSON(filepath.Join(root, "dist", "codex", ".codex", "hooks.json"), &payload); err != nil {
		return err
	}
	if len(payload.Hooks.SessionStart) != 1 || payload.Hooks.SessionStart[0].Matcher != "startup|resume|clear|compact" {
		return fmt.Errorf("codex hook %s missing SessionStart context group", hook.id)
	}
	return nil
}

func assertNativeBuildOpenCodeHookSemantics(root string, hook nativeBuildHook) error {
	return assertNativeBuildPluginHookSemantics(filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"), "opencode", hook)
}

func assertNativeBuildAmpHookSemantics(root string, hook nativeBuildHook) error {
	if hook.id == "detect-linear-magic" {
		hooks, err := readNativeBuildPluginPreToolHooks(filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"))
		if err != nil {
			return err
		}
		for _, entries := range hooks {
			for _, entry := range entries {
				if entry.ID == hook.id {
					return fmt.Errorf("amp hook %s present without trustworthy root-session identity", hook.id)
				}
			}
		}
		return nil
	}
	return assertNativeBuildPluginHookSemantics(filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"), "amp", hook)
}

func assertNativeBuildPluginHookSemantics(path string, target string, hook nativeBuildHook) error {
	hooks, err := readNativeBuildPluginPreToolHooks(path)
	if err != nil {
		return err
	}
	for _, entries := range hooks {
		for _, entry := range entries {
			if entry.ID == hook.id {
				if entry.FailClosed != hook.failClosed {
					return fmt.Errorf("%s hook %s failClosed = %v, want %v", target, hook.id, entry.FailClosed, hook.failClosed)
				}
				if entry.If != hook.ifCondition {
					return fmt.Errorf("%s hook %s if = %q, want %q", target, hook.id, entry.If, hook.ifCondition)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("%s hook %s missing", target, hook.id)
}

func readNativeBuildPluginPreToolHooks(path string) (map[string][]nativeAmpHookEntry, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	startMarker := "const preToolHooks: Record<string, HookEntry[]> = "
	start := strings.Index(string(body), startMarker)
	if start < 0 {
		return nil, fmt.Errorf("%s missing preToolHooks", path)
	}
	rest := string(body)[start+len(startMarker):]
	end := strings.Index(rest, ";\n\nconst postToolHooks")
	if end < 0 {
		return nil, fmt.Errorf("%s missing postToolHooks delimiter", path)
	}
	var hooks map[string][]nativeAmpHookEntry
	if err := json.Unmarshal([]byte(rest[:end]), &hooks); err != nil {
		return nil, err
	}
	return hooks, nil
}

func readNativeBuildJSON(path string, out any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func findNativeBuildGenericHook(groups []struct {
	Hooks []map[string]any `json:"hooks"`
}, key string, value string) (map[string]any, bool) {
	for _, group := range groups {
		for _, entry := range group.Hooks {
			if got, ok := entry[key].(string); ok && got == value {
				return entry, true
			}
		}
	}
	return nil, false
}

func nativeBuildGenericBool(entry map[string]any, key string) bool {
	value, _ := entry[key].(bool)
	return value
}

func readBuildFileString(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}

func buildAmpPluginForDelegateTests(t *testing.T) string {
	t.Helper()
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build", "--target", "amp"}); err != nil {
		t.Fatalf("build --target amp error = %v\n%s", err, stdout.String())
	}
	return readBuildFileString(t, filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"))
}

func extractAmpDelegateToolList(t *testing.T, plugin string, ident string) []string {
	t.Helper()
	marker := "const " + ident + " = ["
	start := strings.Index(plugin, marker)
	if start < 0 {
		t.Fatalf("plugin missing %s", ident)
	}
	rest := plugin[start+len(marker):]
	end := strings.Index(rest, "]")
	if end < 0 {
		t.Fatalf("%s unterminated", ident)
	}
	var tools []string
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "'") || !strings.HasSuffix(line, "'") {
			t.Fatalf("%s contains non-literal %q", ident, line)
		}
		tools = append(tools, strings.Trim(line, "'"))
	}
	return tools
}

func extractAmpDelegateBlock(t *testing.T, plugin string, startMarker string, endMarker string) string {
	t.Helper()
	start := strings.Index(plugin, startMarker)
	if start < 0 {
		t.Fatalf("plugin missing %q", startMarker)
	}
	rest := plugin[start:]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func sameStringSlice(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type ampDelegateHarnessAgent struct {
	Model          string   `json:"model"`
	Reasoning      string   `json:"reasoning"`
	Tools          []string `json:"tools"`
	Prompt         string   `json:"prompt"`
	ParentThreadID string   `json:"parentThreadID"`
	Executor       string   `json:"executor"`
	GitInvoked     bool     `json:"gitInvoked"`
	ShellInvoked   bool     `json:"shellInvoked"`
}

type ampDelegateHarnessResult struct {
	OK    bool                    `json:"ok"`
	Error string                  `json:"error"`
	Grok  ampDelegateHarnessAgent `json:"grok"`
	Luna  ampDelegateHarnessAgent `json:"luna"`
}

func assertAmpDelegateCompatibilityFailure(t *testing.T, got string, cause string) {
	t.Helper()
	for _, want := range []string{
		"compatibility failure",
		"amp plugins show-agent-options",
		"Do not substitute",
		"no silent fallback",
		"no local fallback",
		cause,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compatibility diagnostic = %q, want %q", got, want)
		}
	}
}

func runAmpDelegateHarness(t *testing.T, plugin string, payload map[string]any) ampDelegateHarnessResult {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required to execute the generated Amp plugin contract")
	}
	dir := t.TempDir()
	pluginPath := filepath.Join(dir, "loaf.ts")
	harnessPath := filepath.Join(dir, "harness.mjs")
	writeFile(t, pluginPath, plugin)
	writeFile(t, harnessPath, ampDelegateHarnessSource())
	input, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(harness payload) error = %v", err)
	}
	cmd := exec.Command("node", harnessPath, pluginPath)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("amp delegate harness error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	var result ampDelegateHarnessResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(harness result) error = %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	return result
}

func ampDelegateHarnessSource() string {
	return `import { readFileSync } from 'node:fs';
import { realpathSync, statSync } from 'node:fs';
import { isAbsolute } from 'node:path';

const pluginPath = process.argv[2];
const payload = JSON.parse(readFileSync(0, 'utf8'));
const source = readFileSync(pluginPath, 'utf8');

function requiredLiteral(name) {
  const match = source.match(new RegExp("const " + name + " = '([^']+)'"));
  if (!match) throw new Error('missing ' + name);
  return match[1];
}

function requiredTools(name) {
  const marker = 'const ' + name + ' = [';
  const start = source.indexOf(marker);
  if (start < 0) throw new Error('missing ' + name);
  const end = source.indexOf('];', start);
  return source.slice(start + marker.length, end).split('\n').map((line) => line.trim().replace(/[',]/g, '')).filter(Boolean);
}

function extractBlock(startMarker, endMarker) {
  const start = source.indexOf(startMarker);
  if (start < 0) throw new Error('missing ' + startMarker);
  const end = source.indexOf(endMarker, start);
  if (end < 0) throw new Error('missing ' + endMarker);
  return source.slice(start, end);
}

const GROK_MODEL = requiredLiteral('GROK_MODEL');
const LUNA_MODEL = requiredLiteral('LUNA_MODEL');
const GROK_IMPL_TOOLS = requiredTools('GROK_IMPL_TOOLS');
const LUNA_REVIEW_TOOLS = requiredTools('LUNA_REVIEW_TOOLS');
const grokExecute = extractBlock("name: 'delegate_grok_implementation'", "registerPinnedTool(amp, 'luna-review'");
const lunaExecute = extractBlock("name: 'delegate_luna_review'", "export default function");
if (!grokExecute.includes("required: ['workdir', 'prompt']") || !lunaExecute.includes("required: ['workdir', 'diff', 'prompt']")) {
  throw new Error('generated execute blocks missing required fields');
}

function requireNonEmptyString(input, name) {
  if (typeof input !== 'string' || input.trim().length === 0) {
    throw new Error(name + ' must be a non-empty string');
  }
  return input;
}

function requireWorkdir(input) {
  const requested = requireNonEmptyString(input, 'workdir');
  if (!isAbsolute(requested)) {
    throw new Error('workdir must be an absolute path');
  }
  const workdir = realpathSync(requested);
  if (!statSync(workdir).isDirectory()) {
    throw new Error('workdir must be an existing directory');
  }
  return workdir;
}

function delegateCompatibilityFailure(role, model, tools, cause) {
  const detail = cause instanceof Error ? cause.message : cause ? String(cause) : '';
  return new Error([
    role + ' compatibility failure: pinned model ' + model + ' or required tools [' + tools.join(', ') + '] are unavailable.',
    detail,
    'Do not substitute a model, reasoning level, broader capability set, or local execution by the orchestrating model.',
    'There is no silent fallback and no local fallback.',
    'Run amp plugins show-agent-options --json and confirm the pinned model id and each required built-in tool name before retrying.',
  ].filter(Boolean).join(' '));
}

function createPinnedAgent(amp, role, model, tools, config) {
  try {
    return amp.createAgent(config);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerPinnedAgentMode(amp, role, model, tools, definition) {
  try {
    amp.registerAgentMode(definition);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerPinnedTool(amp, role, model, tools, definition) {
  try {
    amp.registerTool(definition);
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

async function runPinnedAgent(agent, role, model, tools, prompt, ctx) {
  try {
    return await agent.run(prompt, {
      parentThreadID: ctx.thread.id,
      executor: 'local',
      timeoutMs: 3600000,
    });
  } catch (cause) {
    throw delegateCompatibilityFailure(role, model, tools, cause);
  }
}

function registerDelegatedAgents(amp) {
  const grokImplementer = createPinnedAgent(amp, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, {
    name: 'grok-implementation-agent',
    model: 'xai/grok-4.6',
    reasoningEffort: 'high',
    tools: GROK_IMPL_TOOLS,
  });
  const lunaReviewer = createPinnedAgent(amp, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, {
    name: 'luna-review-agent',
    model: 'openai/gpt-5.6-luna',
    reasoningEffort: 'xhigh',
    tools: LUNA_REVIEW_TOOLS,
  });
  registerPinnedAgentMode(amp, 'loaf-medium', 'openai/gpt-5.6-sol', ['Read'], { key: 'loaf-medium', agent: { kind: 'agent-definition' } });
  registerPinnedAgentMode(amp, 'loaf-ultra', 'openai/gpt-5.6-sol', ['Read'], { key: 'loaf-ultra', agent: { kind: 'agent-definition' } });
  registerPinnedTool(amp, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, {
    name: 'delegate_grok_implementation',
    async execute(input, ctx) {
      const workdir = requireWorkdir(input.workdir);
      const brief = requireNonEmptyString(input.prompt, 'prompt');
      const prompt = 'Workspace: ' + workdir + '\n\n' + brief;
      return runPinnedAgent(grokImplementer, 'grok-impl', GROK_MODEL, GROK_IMPL_TOOLS, prompt, ctx);
    },
  });
  registerPinnedTool(amp, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, {
    name: 'delegate_luna_review',
    async execute(input, ctx) {
      const workdir = requireWorkdir(input.workdir);
      const brief = requireNonEmptyString(input.prompt, 'prompt');
      const diff = requireNonEmptyString(input.diff, 'diff');
      const prompt = 'Workspace: ' + workdir + '\n\n' + brief + '\n\nDiff supplied by the delegating agent:\n' + diff;
      return runPinnedAgent(lunaReviewer, 'luna-review', LUNA_MODEL, LUNA_REVIEW_TOOLS, prompt, ctx);
    },
  });
}

function recordFromConfig(config) {
  return {
    model: config.model,
    reasoning: config.reasoningEffort,
    tools: [...(config.tools || [])],
    prompt: '',
    parentThreadID: '',
    executor: '',
    gitInvoked: false,
    shellInvoked: false,
  };
}

function createStub(kind) {
  const state = { modes: new Set(), tools: new Map(), grok: null, luna: null };
  function agentFor(config) {
    const record = recordFromConfig(config);
    if (config.name === 'grok-implementation-agent') state.grok = record;
    if (config.name === 'luna-review-agent') state.luna = record;
    return {
      definition: { kind: 'agent-definition', name: config.name },
      async run(prompt, options = {}) {
        if (kind === 'run-failure') throw new Error('agent.run unavailable');
        record.prompt = prompt;
        record.parentThreadID = options.parentThreadID || '';
        record.executor = options.executor || '';
        const lowered = String(prompt).toLowerCase();
        if (/\bgit\s+(diff|status|add|commit|push|restore)\b/.test(lowered)) record.gitInvoked = true;
        if (/\b(shell_command|execfile)\b/.test(lowered)) record.shellInvoked = true;
        return { threadID: 'T-' + config.name, text: config.name + ' ok' };
      },
    };
  }
  return {
    amp: {
      createAgent(config) {
        if (kind === 'create-failure') throw new Error('createAgent unavailable');
        return agentFor(config);
      },
      registerAgentMode(definition) {
        if (state.modes.has(definition.key) || kind === 'duplicate-registration') {
          throw new Error('already registered mode ' + definition.key);
        }
        state.modes.add(definition.key);
      },
      registerTool(definition) {
        if (state.tools.has(definition.name)) throw new Error('already registered tool ' + definition.name);
        state.tools.set(definition.name, definition);
      },
    },
    state,
  };
}

const { amp, state } = createStub(payload.case);
const ctx = { thread: { id: 'T-parent' } };
try {
  registerDelegatedAgents(amp);
  if (payload.case === 'happy' || payload.case === 'run-failure' || payload.case === 'invalid-prompt' || payload.case === 'invalid-workdir') {
    await state.tools.get('delegate_grok_implementation').execute({ workdir: payload.workdir, prompt: payload.prompt }, ctx);
  }
  if (payload.case === 'happy' || payload.case === 'missing-diff') {
    await state.tools.get('delegate_luna_review').execute({ workdir: payload.workdir, prompt: payload.prompt, diff: payload.diff }, ctx);
  }
  console.log(JSON.stringify({ ok: true, error: '', grok: state.grok, luna: state.luna }));
} catch (error) {
  console.log(JSON.stringify({
    ok: false,
    error: error instanceof Error ? error.message : String(error),
    grok: state.grok,
    luna: state.luna,
  }));
}
`
}
