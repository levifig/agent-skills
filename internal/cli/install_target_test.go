package cli

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func codexInstallTestOperations(t *testing.T, projectRoot string) *codexRuleInstallOperations {
	t.Helper()
	workspace, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	trusted, err := os.CreateTemp(workspace, ".loaf-codex-test-*")
	if err != nil {
		t.Fatalf("CreateTemp(trusted loaf) error = %v", err)
	}
	trustedPath := trusted.Name()
	if err := trusted.Close(); err != nil {
		t.Fatalf("Close(trusted loaf) error = %v", err)
	}
	if err := os.Chmod(trustedPath, 0o755); err != nil {
		t.Fatalf("Chmod(trusted loaf) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(trustedPath) })
	pathOnly := filepath.Join(projectRoot, "empty-path")
	if err := os.MkdirAll(pathOnly, 0o755); err != nil {
		t.Fatalf("MkdirAll(empty PATH) error = %v", err)
	}
	t.Setenv("PATH", pathOnly)
	return &codexRuleInstallOperations{
		lookPath:       func(string) (string, error) { return trustedPath, nil },
		forbiddenRoots: []string{projectRoot},
	}
}

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

func TestInstallTargetAdapterManifestPreservesForeignConvergesAndRejectsTampering(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "opencode")
	config := filepath.Join(root, "config", "opencode")
	pluginSource := filepath.Join(dist, "plugins", "hooks.ts")
	pluginDest := filepath.Join(config, "plugins", "hooks.ts")
	foreign := filepath.Join(config, "plugins", "company.ts")
	writeInstallFile(t, pluginSource, "export const generation = 1;\n")
	writeInstallFile(t, foreign, "export const company = true;\n")
	writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
		"id":          "runtime-plugin",
		"kind":        "plugin",
		"source_path": "plugins/hooks.ts",
		"destination": "plugins/hooks.ts",
		"sha256":      sha256Hex("export const generation = 1;\n"),
	}})

	options := targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("first OpenCode install error = %v", err)
	}
	assertInstallFile(t, pluginDest, "export const generation = 1;\n")
	assertInstallFile(t, foreign, "export const company = true;\n")
	if _, err := os.Stat(filepath.Join(config, ".loaf-managed-target.json")); err != nil {
		t.Fatalf("managed target manifest stat error = %v", err)
	}

	writeInstallFile(t, pluginSource, "export const generation = 2;\n")
	writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
		"id":          "runtime-plugin",
		"kind":        "plugin",
		"source_path": "plugins/hooks.ts",
		"destination": "plugins/hooks.ts",
		"sha256":      sha256Hex("export const generation = 2;\n"),
	}})
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("same-version OpenCode convergence error = %v", err)
	}
	assertInstallFile(t, pluginDest, "export const generation = 2;\n")

	writeInstallFile(t, pluginDest, "// local edit\n")
	if err := installTargetDistribution(options); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("tampered OpenCode install error = %v, want modified-content conflict", err)
	}
	assertInstallFile(t, pluginDest, "// local edit\n")

	writeInstallFile(t, pluginDest, "export const generation = 2;\n")
	if err := os.Remove(pluginSource); err != nil {
		t.Fatalf("Remove(plugin source) error = %v", err)
	}
	writeTestTargetAdapterManifest(t, dist, "opencode", nil)
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("stale OpenCode plugin removal error = %v", err)
	}
	assertInstallPathMissing(t, pluginDest)
	assertInstallFile(t, foreign, "export const company = true;\n")
}

// Cursor's support files stay whole-file artifacts while its entries converge
// one at a time: the user's entry is untouched, Loaf's own entry is brought to
// the shipped shape wherever it drifted, and no divergence refuses the file.
func TestInstallTargetAdapterManifestReconcilesCursorEntriesAndOwnsSupportFiles(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "cursor")
	config := filepath.Join(root, "cursor")
	desired := map[string]any{"command": "loaf task refresh", "matcher": "Edit|Write", loafHookMarker: true}
	generatedHooks := `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf task refresh","matcher":"Edit|Write","loaf-managed":true}]}}`
	existingHooks := `{"version":1,"hooks":{"PostToolUse":[{"command":"user hook"},{"command":"loaf task refresh","matcher":"Bash","loaf-managed":true}]}}`
	writeInstallFile(t, filepath.Join(dist, "hooks.json"), generatedHooks)
	writeInstallFile(t, filepath.Join(dist, "hooks", "post-tool", "managed.sh"), "#!/bin/sh\necho managed\n")
	writeInstallFile(t, filepath.Join(config, "hooks.json"), existingHooks)
	writeInstallFile(t, filepath.Join(config, "hooks", "company.sh"), "#!/bin/sh\necho company\n")
	writeTestTargetAdapterManifest(t, dist, "cursor", []map[string]string{
		{"id": "hook-file:hooks/post-tool/managed.sh", "kind": "hook-file", "source_path": "hooks/post-tool/managed.sh", "destination": "hooks/post-tool/managed.sh", "sha256": sha256Hex("#!/bin/sh\necho managed\n")},
	})
	installTestHookCatalog(t, dist, "cursor", []hookCatalogSource{{
		event: "PostToolUse", hookID: "generate-task-board", typeName: "command", command: "loaf task refresh", template: desired,
	}})
	options := targetInstallOptions{Target: "cursor", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home, HookState: installTestHookState(t)}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Cursor adapter install error = %v", err)
	}
	hooks := readInstallHooks(t, filepath.Join(config, "hooks.json"))
	if len(hooks.Hooks["PostToolUse"]) != 2 || hooks.Hooks["PostToolUse"][0]["command"] != "user hook" {
		t.Fatalf("Cursor hooks = %#v, want the user entry preserved in place", hooks.Hooks)
	}
	if hooks.Hooks["PostToolUse"][1]["matcher"] != "Edit|Write" {
		t.Fatalf("Cursor Loaf entry = %#v, want the drifted entry converged", hooks.Hooks["PostToolUse"][1])
	}
	assertInstallFile(t, filepath.Join(config, "hooks", "company.sh"), "#!/bin/sh\necho company\n")

	hooks.Hooks["PostToolUse"][1]["command"] = "loaf task refresh --advisory"
	body, err := json.Marshal(hooks)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(config, "hooks.json"), string(body))
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("weakened Cursor entry install error = %v, want convergence rather than a refusal", err)
	}
	hooks = readInstallHooks(t, filepath.Join(config, "hooks.json"))
	if hooks.Hooks["PostToolUse"][1]["command"] != "loaf task refresh" {
		t.Fatalf("Cursor Loaf entry = %#v, want the weakened entry converged back", hooks.Hooks["PostToolUse"][1])
	}
}

func TestInstallTargetAdapterManifestCanonicalizesCodexExecutable(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "codex")
	config := filepath.Join(root, "reported-config")
	codexHome := filepath.Join(root, "codex-home")
	operations := codexInstallTestOperations(t, root)
	generatedHooks := `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook","commandWindows":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), generatedHooks)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user hook"}]}]}}`)
	writeTestTargetAdapterManifest(t, dist, "codex", nil)
	installTestHookCatalog(t, dist, "codex", []hookCatalogSource{{
		event:    "SessionStart",
		hookID:   "session-start-loaf",
		typeName: "command",
		command:  codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix,
		template: map[string]any{
			"matcher": codexJournalHookMatcher,
			"hooks": []any{map[string]any{
				"type":           "command",
				"command":        codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix,
				"commandWindows": codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix,
			}},
		},
	}})
	options := targetInstallOptions{Target: "codex", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home, CodexHome: codexHome, CodexRuleOperations: operations, ProjectRoot: root, HookState: installTestHookState(t)}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Codex adapter install error = %v", err)
	}
	installed := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	if len(installed.Hooks["SessionStart"]) != 2 {
		t.Fatalf("Codex hooks = %#v, want user plus Loaf group", installed.Hooks)
	}
	if strings.Contains(string(readFileBytes(t, filepath.Join(codexHome, "hooks.json"))), codexJournalExecutablePlaceholder) {
		t.Fatal("installed Codex hooks retained executable placeholder")
	}
	// The rendered entry is recognized back as the identity that wrote it: a
	// second install converges rather than adding a second group.
	after := string(readFileBytes(t, filepath.Join(codexHome, "hooks.json")))
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("second Codex adapter install error = %v", err)
	}
	if got := string(readFileBytes(t, filepath.Join(codexHome, "hooks.json"))); got != after {
		t.Fatalf("second install rewrote the file:\nwant %s\ngot  %s", after, got)
	}
}

func TestInstallTargetAdapterManifestMigratesAndRetiresAmpPlugin(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "amp")
	sourcePath := filepath.Join(dist, ".amp", "plugins", "loaf.ts")
	destination := filepath.Join(config, "plugins", "loaf.ts")
	oldPlugin := "/**\n * Amp Plugin - Agent Skills Hooks\n * Auto-generated by loaf build system\n */\nold\n"
	newPlugin := "/**\n * Amp Plugin - Agent Skills Hooks\n * Auto-generated by loaf build system\n */\nnew\n"
	writeInstallFile(t, sourcePath, newPlugin)
	writeInstallFile(t, destination, oldPlugin)
	writeInstallFile(t, filepath.Join(config, "plugins", "company.ts"), "company\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{{
		"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex(newPlugin),
	}})
	options := targetInstallOptions{Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Amp migration error = %v", err)
	}
	assertInstallFile(t, destination, newPlugin)
	assertInstallFile(t, filepath.Join(config, "plugins", "company.ts"), "company\n")

	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	writeTestTargetAdapterManifest(t, dist, "amp", nil)
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Amp stale plugin retirement error = %v", err)
	}
	assertInstallPathMissing(t, destination)
	assertInstallFile(t, filepath.Join(config, "plugins", "company.ts"), "company\n")
}

func TestInstallTargetAdapterManifestAdoptsExactAmpModesPredecessor(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "amp")
	hookSource := filepath.Join(dist, ".amp", "plugins", "loaf.ts")
	modesSource := filepath.Join(dist, ".amp", "plugins", "loaf-modes.ts")
	hookDest := filepath.Join(config, "plugins", "loaf.ts")
	modesDest := filepath.Join(config, "plugins", "loaf-modes.ts")
	foreign := filepath.Join(config, "plugins", "company.ts")
	hookBody := "export const loafHooks = 1;\n"
	desiredModes := "export const loafModes = desired;\n"
	predecessor := testAmpModesPredecessor(t)
	if digest := sha256Hex(predecessor); digest != ampModesPluginPredecessorSHA256 {
		t.Fatalf("predecessor digest = %s, want %s", digest, ampModesPluginPredecessorSHA256)
	}
	writeInstallFile(t, hookSource, hookBody)
	writeInstallFile(t, modesSource, desiredModes)
	writeInstallFile(t, modesDest, predecessor)
	writeInstallFile(t, foreign, "export const company = true;\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{
		{"id": ampModesPluginArtifactID, "kind": "plugin", "source_path": ampModesPluginSourcePath, "destination": ampModesPluginDestination, "sha256": sha256Hex(desiredModes)},
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex(hookBody)},
	})
	options := targetInstallOptions{Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home}

	decisions, err := planTargetAdapterArtifacts(options)
	if err != nil {
		t.Fatalf("planTargetAdapterArtifacts error = %v", err)
	}
	modesDecision := findArtifactPlanDecision(t, decisions, ampModesPluginArtifactID)
	if modesDecision.Action != planActionUpdate {
		t.Fatalf("predecessor plan action = %#v, want update", modesDecision)
	}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Amp predecessor adopt error = %v", err)
	}
	assertInstallFile(t, modesDest, desiredModes)
	assertInstallFile(t, hookDest, hookBody)
	assertInstallFile(t, foreign, "export const company = true;\n")
	installed, err := readTargetAdapterManifest(filepath.Join(config, targetInstallManifestFile))
	if err != nil {
		t.Fatalf("read installed Amp ownership error = %v", err)
	}
	owned, ok := targetAdapterArtifactsByID(installed.Artifacts)[ampModesPluginArtifactID]
	if !ok || owned.SHA256 != sha256Hex(desiredModes) {
		t.Fatalf("installed Amp modes ownership = %#v, want current desired digest", owned)
	}
}

func TestInstallTargetAdapterManifestRefusesOneByteAmpModesPredecessor(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "amp")
	hookSource := filepath.Join(dist, ".amp", "plugins", "loaf.ts")
	modesSource := filepath.Join(dist, ".amp", "plugins", "loaf-modes.ts")
	hookDest := filepath.Join(config, "plugins", "loaf.ts")
	modesDest := filepath.Join(config, "plugins", "loaf-modes.ts")
	foreign := filepath.Join(config, "plugins", "company.ts")
	hookBody := "export const loafHooks = 1;\n"
	desiredModes := "export const loafModes = desired;\n"
	modified := testAmpModesPredecessor(t) + " "
	if digest := sha256Hex(modified); digest == ampModesPluginPredecessorSHA256 {
		t.Fatal("one-byte modification still matched the closed predecessor digest")
	}
	writeInstallFile(t, hookSource, hookBody)
	writeInstallFile(t, modesSource, desiredModes)
	writeInstallFile(t, modesDest, modified)
	writeInstallFile(t, foreign, "export const company = true;\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{
		{"id": ampModesPluginArtifactID, "kind": "plugin", "source_path": ampModesPluginSourcePath, "destination": ampModesPluginDestination, "sha256": sha256Hex(desiredModes)},
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex(hookBody)},
	})
	options := targetInstallOptions{Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home}

	decisions, err := planTargetAdapterArtifacts(options)
	if err != nil {
		t.Fatalf("planTargetAdapterArtifacts error = %v", err)
	}
	modesDecision := findArtifactPlanDecision(t, decisions, ampModesPluginArtifactID)
	if modesDecision.Action != planActionConflict {
		t.Fatalf("modified predecessor plan action = %#v, want conflict", modesDecision)
	}
	if err := installTargetDistribution(options); err == nil || !strings.Contains(err.Error(), "not managed by Loaf") {
		t.Fatalf("modified predecessor install error = %v, want unmanaged collision refusal", err)
	}
	assertInstallFile(t, modesDest, modified)
	assertInstallPathMissing(t, hookDest)
	assertInstallFile(t, foreign, "export const company = true;\n")
	assertInstallPathMissing(t, filepath.Join(config, targetInstallManifestFile))
}

func TestInstallTargetAdapterManifestManagesIndependentAmpPlugins(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "amp")
	hookSource := filepath.Join(dist, ".amp", "plugins", "loaf.ts")
	modesSource := filepath.Join(dist, ".amp", "plugins", "loaf-modes.ts")
	hookDest := filepath.Join(config, "plugins", "loaf.ts")
	modesDest := filepath.Join(config, "plugins", "loaf-modes.ts")
	foreign := filepath.Join(config, "plugins", "company.ts")
	hookBody := "export const loafHooks = 1;\n"
	modesBody := "export const loafModes = 1;\n"
	writeInstallFile(t, hookSource, hookBody)
	writeInstallFile(t, modesSource, modesBody)
	writeInstallFile(t, foreign, "export const company = true;\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex(hookBody)},
		{"id": "plugin:.amp/plugins/loaf-modes.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf-modes.ts", "destination": "plugins/loaf-modes.ts", "sha256": sha256Hex(modesBody)},
	})
	options := targetInstallOptions{Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home}

	writeInstallFile(t, modesDest, "export const loafModes = local;\n")
	if err := installTargetDistribution(options); err == nil || !strings.Contains(err.Error(), "not managed by Loaf") {
		t.Fatalf("Amp modes collision error = %v, want unmanaged collision refusal", err)
	}
	assertInstallPathMissing(t, hookDest)
	assertInstallFile(t, modesDest, "export const loafModes = local;\n")
	assertInstallFile(t, foreign, "export const company = true;\n")

	writeInstallFile(t, modesDest, modesBody)
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("Amp two-plugin install with exact-digest adoption error = %v", err)
	}
	assertInstallFile(t, hookDest, hookBody)
	assertInstallFile(t, modesDest, modesBody)
	assertInstallFile(t, foreign, "export const company = true;\n")

	writeInstallFile(t, modesDest, "export const loafModes = edited;\n")
	if err := installTargetDistribution(options); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("Amp modes modified-content error = %v, want modified-content conflict", err)
	}
	assertInstallFile(t, modesDest, "export const loafModes = edited;\n")
	assertInstallFile(t, hookDest, hookBody)
	assertInstallFile(t, foreign, "export const company = true;\n")
}

func TestInstallTargetAdapterManifestRollsBackPartialAmpPublication(t *testing.T) {
	root := realpath(t, t.TempDir())
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "config", "amp")
	writeInstallFile(t, filepath.Join(dist, ".amp", "plugins", "loaf.ts"), "hooks\n")
	writeInstallFile(t, filepath.Join(dist, ".amp", "plugins", "loaf-modes.ts"), "modes\n")
	writeInstallFile(t, filepath.Join(config, "plugins", "company.ts"), "company\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("hooks\n")},
		{"id": "plugin:.amp/plugins/loaf-modes.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf-modes.ts", "destination": "plugins/loaf-modes.ts", "sha256": sha256Hex("modes\n")},
	})
	modesDest := filepath.Join(config, "plugins", "loaf-modes.ts")
	publishedModes := 0
	options := targetInstallOptions{
		Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"),
		TargetAdapterOps: &targetAdapterInstallOperations{beforeArtifact: func(id string) error {
			if id == "plugin:.amp/plugins/loaf.ts" {
				assertInstallFile(t, modesDest, "modes\n")
				publishedModes++
				return os.ErrPermission
			}
			return nil
		}},
	}
	if err := installTargetDistribution(options); !os.IsPermission(err) {
		t.Fatalf("Amp partial publication error = %v, want permission injection", err)
	}
	if publishedModes != 1 {
		t.Fatalf("published loaf-modes.ts observations = %d, want 1 before loaf.ts failed", publishedModes)
	}
	assertInstallPathMissing(t, filepath.Join(config, "plugins", "loaf.ts"))
	assertInstallPathMissing(t, filepath.Join(config, "plugins", "loaf-modes.ts"))
	assertInstallPathMissing(t, filepath.Join(config, targetInstallManifestFile))
	assertInstallFile(t, filepath.Join(config, "plugins", "company.ts"), "company\n")
}

func TestInstallTargetAdapterManifestRejectsUnrecordedPluginCollisions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      string
		sourcePath  string
		destination string
	}{
		{name: "opencode", target: "opencode", sourcePath: "plugins/hooks.ts", destination: "plugins/hooks.ts"},
		{name: "amp", target: "amp", sourcePath: ".amp/plugins/loaf.ts", destination: "plugins/loaf.ts"},
		{name: "amp-modes", target: "amp", sourcePath: ".amp/plugins/loaf-modes.ts", destination: "plugins/loaf-modes.ts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := realpath(t, t.TempDir())
			dist := filepath.Join(root, "dist", tc.target)
			config := filepath.Join(root, "config", tc.target)
			writeInstallFile(t, filepath.Join(dist, filepath.FromSlash(tc.sourcePath)), "Loaf desired\n")
			writeInstallFile(t, filepath.Join(config, filepath.FromSlash(tc.destination)), "user plugin\n")
			writeInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "old\n")
			writeTestTargetAdapterManifest(t, dist, tc.target, []map[string]string{{
				"id": "plugin:" + tc.sourcePath, "kind": "plugin", "source_path": tc.sourcePath, "destination": tc.destination, "sha256": sha256Hex("Loaf desired\n"),
			}})
			err := installTargetDistribution(targetInstallOptions{Target: tc.target, DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")})
			if err == nil || !strings.Contains(err.Error(), "not managed by Loaf") {
				t.Fatalf("%s collision error = %v", tc.name, err)
			}
			assertInstallFile(t, filepath.Join(config, filepath.FromSlash(tc.destination)), "user plugin\n")
		})
	}
}

func TestInstallTargetAdapterManifestRejectsMalformedInstalledOwnership(t *testing.T) {
	root := realpath(t, t.TempDir())
	dist := filepath.Join(root, "dist", "amp")
	config := filepath.Join(root, "config", "amp")
	writeInstallFile(t, filepath.Join(dist, ".amp", "plugins", "loaf.ts"), "desired\n")
	writeTestTargetAdapterManifest(t, dist, "amp", []map[string]string{{
		"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("desired\n"),
	}})
	writeInstallFile(t, filepath.Join(config, targetInstallManifestFile), `{"version":1,"version":1}`)
	err := installTargetDistribution(targetInstallOptions{Target: "amp", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("malformed installed ownership error = %v", err)
	}
	assertInstallPathMissing(t, filepath.Join(config, "plugins", "loaf.ts"))
}

func TestInstallTargetAdapterManifestRejectsSymlinkedDestinationParents(t *testing.T) {
	root := realpath(t, t.TempDir())
	dist := filepath.Join(root, "dist", "opencode")
	config := filepath.Join(root, "config", "opencode")
	outside := filepath.Join(root, "outside")
	writeInstallFile(t, filepath.Join(dist, "plugins", "hooks.ts"), "desired\n")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(config, "plugins")); err != nil {
		t.Fatal(err)
	}
	writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
		"id": "plugin:plugins/hooks.ts", "kind": "plugin", "source_path": "plugins/hooks.ts", "destination": "plugins/hooks.ts", "sha256": sha256Hex("desired\n"),
	}})
	err := installTargetDistribution(targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")})
	if err == nil || !strings.Contains(err.Error(), "not a real directory") {
		t.Fatalf("symlinked destination parent error = %v", err)
	}
	assertInstallPathMissing(t, filepath.Join(outside, "hooks.ts"))
}

func TestInstallTargetAdapterManifestBindsConcreteArtifactModes(t *testing.T) {
	t.Run("post-build source chmod drift", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		source := filepath.Join(dist, "plugins", "hooks.ts")
		writeInstallFile(t, source, "plugin\n")
		if err := os.Chmod(source, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
			"id": "plugin:hooks", "kind": "plugin", "source_path": "plugins/hooks.ts", "destination": "plugins/hooks.ts", "sha256": sha256Hex("plugin\n"),
		}})
		if err := os.Chmod(source, 0o644); err != nil {
			t.Fatal(err)
		}
		err := installTargetDistribution(targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")})
		if err == nil || !strings.Contains(err.Error(), "mode 0644 does not match manifest mode 0755") {
			t.Fatalf("source chmod drift error = %v", err)
		}
		assertInstallPathMissing(t, filepath.Join(config, "plugins", "hooks.ts"))
	})

	t.Run("installed chmod drift", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		source := filepath.Join(dist, "plugins", "hooks.ts")
		destination := filepath.Join(config, "plugins", "hooks.ts")
		writeInstallFile(t, source, "plugin\n")
		if err := os.Chmod(source, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
			"id": "plugin:hooks", "kind": "plugin", "source_path": "plugins/hooks.ts", "destination": "plugins/hooks.ts", "sha256": sha256Hex("plugin\n"),
		}})
		options := targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")}
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		assertInstallMode(t, destination, 0o755)
		if err := os.Chmod(destination, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := installTargetDistribution(options); err == nil || !strings.Contains(err.Error(), "modified") {
			t.Fatalf("installed chmod drift error = %v", err)
		}
		assertInstallMode(t, destination, 0o644)
	})

	t.Run("executable hook and reconciled hooks-file modes", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "cursor")
		config := filepath.Join(root, "cursor")
		generatedHooks := `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf task refresh","matcher":"Edit|Write","loaf-managed":true}]}}`
		existingHooks := `{"version":1,"hooks":{"PostToolUse":[{"command":"user hook"}]}}`
		writeInstallFile(t, filepath.Join(dist, "hooks.json"), generatedHooks)
		writeInstallFile(t, filepath.Join(dist, "hooks", "post-tool", "managed.sh"), "#!/bin/sh\necho managed\n")
		if err := os.Chmod(filepath.Join(dist, "hooks", "post-tool", "managed.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeInstallFile(t, filepath.Join(config, "hooks.json"), existingHooks)
		if err := os.Chmod(filepath.Join(config, "hooks.json"), 0o600); err != nil {
			t.Fatal(err)
		}
		writeTestTargetAdapterManifest(t, dist, "cursor", []map[string]string{
			{"id": "hook-file:managed", "kind": "hook-file", "source_path": "hooks/post-tool/managed.sh", "destination": "hooks/post-tool/managed.sh", "sha256": sha256Hex("#!/bin/sh\necho managed\n")},
		})
		installTestHookCatalog(t, dist, "cursor", []hookCatalogSource{{
			event: "PostToolUse", hookID: "generate-task-board", typeName: "command", command: "loaf task refresh",
			template: map[string]any{"command": "loaf task refresh", "matcher": "Edit|Write", loafHookMarker: true},
		}})
		options := targetInstallOptions{Target: "cursor", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"), HookState: installTestHookState(t)}
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		assertInstallMode(t, filepath.Join(config, "hooks", "post-tool", "managed.sh"), 0o755)
		assertInstallMode(t, filepath.Join(config, "hooks.json"), 0o600)
		writeTestTargetAdapterManifest(t, dist, "cursor", nil)
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		assertInstallMode(t, filepath.Join(config, "hooks.json"), 0o600)
	})
}

func TestInstallTargetAdapterManifestDetectsRaceAndRollsBackPublication(t *testing.T) {
	t.Run("post-preflight race", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		source := filepath.Join(dist, "plugins", "hooks.ts")
		destination := filepath.Join(config, "plugins", "hooks.ts")
		writeInstallFile(t, source, "old\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{"id": "plugin:hooks", "kind": "plugin", "source_path": "plugins/hooks.ts", "destination": "plugins/hooks.ts", "sha256": sha256Hex("old\n")}})
		options := targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")}
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		writeInstallFile(t, source, "new\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{"id": "plugin:hooks", "kind": "plugin", "source_path": "plugins/hooks.ts", "destination": "plugins/hooks.ts", "sha256": sha256Hex("new\n")}})
		options.TargetAdapterOps = &targetAdapterInstallOperations{beforePublish: func() error {
			writeInstallFile(t, destination, "raced\n")
			return nil
		}}
		err := installTargetDistribution(options)
		if err == nil || !strings.Contains(err.Error(), "changed during install") {
			t.Fatalf("race error = %v", err)
		}
		assertInstallFile(t, destination, "raced\n")
	})

	t.Run("pending existing publication changes", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		aSource := filepath.Join(dist, "plugins", "a.ts")
		bSource := filepath.Join(dist, "plugins", "b.ts")
		aDestination := filepath.Join(config, "plugins", "a.ts")
		bDestination := filepath.Join(config, "plugins", "b.ts")
		writeInstallFile(t, aSource, "old a\n")
		writeInstallFile(t, bSource, "old b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("old a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("old b\n")},
		})
		options := targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")}
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		writeInstallFile(t, aSource, "new a\n")
		writeInstallFile(t, bSource, "new b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("new a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("new b\n")},
		})
		options.TargetAdapterOps = &targetAdapterInstallOperations{beforeArtifact: func(id string) error {
			if id == "plugin:b" {
				writeInstallFile(t, bDestination, "raced b\n")
			}
			return nil
		}}
		err := installTargetDistribution(options)
		if err == nil || !strings.Contains(err.Error(), "changed during install") {
			t.Fatalf("pending publication race error = %v", err)
		}
		assertInstallFile(t, aDestination, "old a\n")
		assertInstallFile(t, bDestination, "raced b\n")
	})

	t.Run("foreign file appears at pending absent destination", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		writeInstallFile(t, filepath.Join(dist, "plugins", "a.ts"), "a\n")
		writeInstallFile(t, filepath.Join(dist, "plugins", "b.ts"), "b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("b\n")},
		})
		options := targetInstallOptions{
			Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"),
			TargetAdapterOps: &targetAdapterInstallOperations{beforeArtifact: func(id string) error {
				if id == "plugin:b" {
					writeInstallFile(t, filepath.Join(config, "plugins", "b.ts"), "foreign b\n")
				}
				return nil
			}},
		}
		err := installTargetDistribution(options)
		if err == nil || !strings.Contains(err.Error(), "changed during install") {
			t.Fatalf("appeared destination race error = %v", err)
		}
		assertInstallPathMissing(t, filepath.Join(config, "plugins", "a.ts"))
		assertInstallFile(t, filepath.Join(config, "plugins", "b.ts"), "foreign b\n")
	})

	t.Run("pending removal changes", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		writeInstallFile(t, filepath.Join(dist, "plugins", "a.ts"), "a\n")
		writeInstallFile(t, filepath.Join(dist, "plugins", "b.ts"), "b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("b\n")},
		})
		options := targetInstallOptions{Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home")}
		if err := installTargetDistribution(options); err != nil {
			t.Fatal(err)
		}
		writeTestTargetAdapterManifest(t, dist, "opencode", nil)
		options.TargetAdapterOps = &targetAdapterInstallOperations{beforeArtifact: func(id string) error {
			if id == "plugin:b" {
				writeInstallFile(t, filepath.Join(config, "plugins", "b.ts"), "raced b\n")
			}
			return nil
		}}
		err := installTargetDistribution(options)
		if err == nil || !strings.Contains(err.Error(), "changed during install") {
			t.Fatalf("pending removal race error = %v", err)
		}
		assertInstallFile(t, filepath.Join(config, "plugins", "a.ts"), "a\n")
		assertInstallFile(t, filepath.Join(config, "plugins", "b.ts"), "raced b\n")
	})

	t.Run("publication rollback", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		writeInstallFile(t, filepath.Join(dist, "plugins", "a.ts"), "a\n")
		writeInstallFile(t, filepath.Join(dist, "plugins", "b.ts"), "b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("b\n")},
		})
		options := targetInstallOptions{
			Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"),
			TargetAdapterOps: &targetAdapterInstallOperations{beforeArtifact: func(id string) error {
				if id == "plugin:b" {
					return os.ErrPermission
				}
				return nil
			}},
		}
		if err := installTargetDistribution(options); !os.IsPermission(err) {
			t.Fatalf("publication error = %v, want permission injection", err)
		}
		assertInstallPathMissing(t, filepath.Join(config, "plugins", "a.ts"))
		assertInstallPathMissing(t, filepath.Join(config, targetInstallManifestFile))
	})

	t.Run("mutation before operation error is rolled back", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		destination := filepath.Join(config, "plugins", "a.ts")
		writeInstallFile(t, filepath.Join(dist, "plugins", "a.ts"), "a\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{{
			"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("a\n"),
		}})
		options := targetInstallOptions{
			Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"),
			TargetAdapterOps: &targetAdapterInstallOperations{afterArtifact: func(id string) error {
				if id == "plugin:a" {
					return os.ErrPermission
				}
				return nil
			}},
		}
		if err := installTargetDistribution(options); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("post-mutation error = %v, want permission", err)
		}
		assertInstallPathMissing(t, destination)
		assertInstallPathMissing(t, filepath.Join(config, targetInstallManifestFile))
	})

	t.Run("rollback failure reports recovery state", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(root, "config", "opencode")
		aDestination := filepath.Join(config, "plugins", "a.ts")
		writeInstallFile(t, filepath.Join(dist, "plugins", "a.ts"), "a\n")
		writeInstallFile(t, filepath.Join(dist, "plugins", "b.ts"), "b\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", []map[string]string{
			{"id": "plugin:a", "kind": "plugin", "source_path": "plugins/a.ts", "destination": "plugins/a.ts", "sha256": sha256Hex("a\n")},
			{"id": "plugin:b", "kind": "plugin", "source_path": "plugins/b.ts", "destination": "plugins/b.ts", "sha256": sha256Hex("b\n")},
		})
		options := targetInstallOptions{
			Target: "opencode", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: filepath.Join(root, "home"),
			TargetAdapterOps: &targetAdapterInstallOperations{
				beforeArtifact: func(id string) error {
					if id == "plugin:b" {
						return os.ErrPermission
					}
					return nil
				},
				restoreSnapshot: func(snapshot targetAdapterSnapshot) error {
					if snapshot.path == aDestination {
						return errors.New("injected restore failure")
					}
					return restoreTargetAdapterSnapshot(snapshot)
				},
			},
		}
		err := installTargetDistribution(options)
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("rollback primary error = %v, want permission", err)
		}
		for _, want := range []string{"rollback failed", aDestination, "injected restore failure", "current state: present mode=0644", "expected state: absent"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rollback error = %v, want %q", err, want)
			}
		}
		assertInstallFile(t, aDestination, "a\n")
		assertInstallPathMissing(t, filepath.Join(config, "plugins", "b.ts"))
		assertInstallPathMissing(t, filepath.Join(config, targetInstallManifestFile))
	})
}

func TestSyncManagedSkillsMigratesV1AndRefusesV2Tampering(t *testing.T) {
	root := t.TempDir()
	src, dest := filepath.Join(root, "dist", "opencode", "skills"), filepath.Join(root, "dest")
	writeInstallFile(t, filepath.Join(src, "foundations", "SKILL.md"), "new\n")
	writeInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "old\n")
	writeInstallFile(t, filepath.Join(dest, loafSkillManifestFile), `{"version":1,"skills":["foundations"]}`)
	if err := syncManagedSkillsDirIfExists(src, dest); err != nil {
		t.Fatalf("v1 migration error = %v", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "new\n")
	manifest := string(readFileBytes(t, filepath.Join(dest, loafSkillManifestFile)))
	if !strings.Contains(manifest, `"version": 2`) || !strings.Contains(manifest, `"sha256"`) {
		t.Fatalf("manifest = %q, want v2 digest", manifest)
	}
	writeInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "tampered\n")
	writeInstallFile(t, filepath.Join(src, "sibling", "SKILL.md"), "sibling\n")
	err := syncManagedSkillsDirIfExists(src, dest)
	if err == nil || !strings.Contains(err.Error(), "was modified") || !strings.Contains(err.Error(), "foundations") {
		t.Fatalf("tampered overwrite error = %v, want foundations modified conflict", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "tampered\n")
	assertInstallFile(t, filepath.Join(dest, "sibling", "SKILL.md"), "sibling\n")
	if err := os.RemoveAll(filepath.Join(src, "foundations")); err != nil {
		t.Fatal(err)
	}
	err = syncManagedSkillsDirIfExists(src, dest)
	if err == nil || !strings.Contains(err.Error(), "was modified") || !strings.Contains(err.Error(), "foundations") {
		t.Fatalf("tampered removal error = %v, want foundations modified conflict", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "tampered\n")
	assertInstallFile(t, filepath.Join(dest, "sibling", "SKILL.md"), "sibling\n")
}

func TestSyncManagedSkillsPreservesForeignAndRejectsInvalidManifests(t *testing.T) {
	root := t.TempDir()
	src, dest := filepath.Join(root, "dist", "opencode", "skills"), filepath.Join(root, "dest")
	writeInstallFile(t, filepath.Join(src, "foundations", "SKILL.md"), "new\n")
	writeInstallFile(t, filepath.Join(src, "sibling", "SKILL.md"), "sibling\n")
	writeInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "foreign\n")
	err := syncManagedSkillsDirIfExists(src, dest)
	if err == nil || !strings.Contains(err.Error(), "not managed") || !strings.Contains(err.Error(), "foundations") {
		t.Fatalf("foreign collision error = %v, want foundations not-managed conflict", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "foreign\n")
	assertInstallFile(t, filepath.Join(dest, "sibling", "SKILL.md"), "sibling\n")
	for _, body := range []string{
		`{"version":2,"skills":[{"name":"../escape","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		`{"version":2,"skills":[{"name":"a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"name":"a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		`{"version":3,"skills":[]}`,
	} {
		writeInstallFile(t, filepath.Join(dest, loafSkillManifestFile), body)
		if _, err := readManagedSkillsState(dest); err == nil {
			t.Fatalf("manifest %s accepted, want rejection", body)
		}
	}
}

func TestHashInstallSkillTreeIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeInstallFile(t, filepath.Join(root, "b", "two.txt"), "two")
	writeInstallFile(t, filepath.Join(root, "a", "one.txt"), "one")
	first, err := hashInstallSkillTree(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hashInstallSkillTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("digests = %q, %q, want equal SHA-256", first, second)
	}
}

func TestHashInstallSkillTreeFramesPathsContentAndPermissions(t *testing.T) {
	makeTree := func(root, path, content string, mode os.FileMode) string {
		writeInstallFile(t, filepath.Join(root, path), content)
		if err := os.Chmod(filepath.Join(root, path), mode); err != nil {
			t.Fatal(err)
		}
		digest, err := hashInstallSkillTree(root)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	firstRoot := filepath.Join(t.TempDir(), "one")
	writeInstallFile(t, filepath.Join(firstRoot, "nested", "alpha.md"), "alpha")
	writeInstallFile(t, filepath.Join(firstRoot, "newline\nbackslash\\name.md"), "adversarial")
	writeInstallFile(t, filepath.Join(firstRoot, "zeta.md"), "zeta")
	first, err := hashInstallSkillTree(firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	secondRoot := filepath.Join(t.TempDir(), "two")
	writeInstallFile(t, filepath.Join(secondRoot, "zeta.md"), "zeta")
	writeInstallFile(t, filepath.Join(secondRoot, "newline\nbackslash\\name.md"), "adversarial")
	writeInstallFile(t, filepath.Join(secondRoot, "nested", "alpha.md"), "alpha")
	second, err := hashInstallSkillTree(secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("creation order independent digests = %q, %q", first, second)
	}
	baseline := makeTree(filepath.Join(t.TempDir(), "baseline"), "nested/SKILL.md", "same", 0o644)
	if baseline == makeTree(filepath.Join(t.TempDir(), "path"), "other/SKILL.md", "same", 0o644) {
		t.Fatal("path change did not change digest")
	}
	if baseline == makeTree(filepath.Join(t.TempDir(), "content"), "nested/SKILL.md", "other", 0o644) {
		t.Fatal("content change did not change digest")
	}
	if baseline == makeTree(filepath.Join(t.TempDir(), "mode"), "nested/SKILL.md", "same", 0o755) {
		t.Fatal("executable-bit change did not change digest")
	}
	rootMode := t.TempDir()
	writeInstallFile(t, filepath.Join(rootMode, "nested", "SKILL.md"), "same")
	firstRootMode, _ := hashInstallSkillTree(rootMode)
	if err := os.Chmod(rootMode, 0o700); err != nil {
		t.Fatal(err)
	}
	secondRootMode, _ := hashInstallSkillTree(rootMode)
	if firstRootMode == secondRootMode {
		t.Fatal("root mode change did not change digest")
	}
	if err := os.Chmod(filepath.Join(rootMode, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	nestedMode, _ := hashInstallSkillTree(rootMode)
	if secondRootMode == nestedMode {
		t.Fatal("nested directory mode change did not change digest")
	}
}

func TestCopyInstallSkillTreePreservesRootAndDirectoryPermissions(t *testing.T) {
	root := t.TempDir()
	source, destination := filepath.Join(root, "source"), filepath.Join(root, "destination")
	writeInstallFile(t, filepath.Join(source, "nested", "SKILL.md"), "skill\n")
	if err := os.Chmod(source, 0o711); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(source, "nested"), 0o751); err != nil {
		t.Fatal(err)
	}
	if err := copyInstallSkillTree(source, destination); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "nested"} {
		sourceInfo, err := os.Stat(filepath.Join(source, path))
		if err != nil {
			t.Fatal(err)
		}
		destinationInfo, err := os.Stat(filepath.Join(destination, path))
		if err != nil {
			t.Fatal(err)
		}
		if sourceInfo.Mode().Perm() != destinationInfo.Mode().Perm() {
			t.Fatalf("mode %q = %o, want %o", path, destinationInfo.Mode().Perm(), sourceInfo.Mode().Perm())
		}
	}
	sourceHash, err := hashInstallSkillTree(source)
	if err != nil {
		t.Fatal(err)
	}
	destinationHash, err := hashInstallSkillTree(destination)
	if err != nil {
		t.Fatal(err)
	}
	if sourceHash != destinationHash {
		t.Fatalf("copied tree hashes = %q, %q", sourceHash, destinationHash)
	}
}

func TestManagedSkillsManifestRejectsUnsafeAndStrictV2Shapes(t *testing.T) {
	dest := t.TempDir()
	manifest := filepath.Join(dest, loafSkillManifestFile)
	writeInstallFile(t, manifest+".target", `{"version":2,"skills":[]}`)
	if err := os.Symlink(manifest+".target", manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := readManagedSkillsState(dest); err == nil {
		t.Fatal("symlink manifest accepted")
	}
	if err := writeManagedSkillsManifest(dest, managedSkillsManifestV2{Version: 2}); err == nil {
		t.Fatal("symlink manifest accepted for write")
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readManagedSkillsState(dest); err == nil {
		t.Fatal("directory manifest accepted")
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"version":2,"skills":null}`,
		`{"version":2,"skills":[{"name":"a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","extra":true}]}`,
		`{"version":2,"skills":[{"name":"a"}]}`,
		`{"version":2,"skills":[{"name":"a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]} {}`,
		`{"version":2,"version":2,"skills":[]}`,
		`{"version":2,"skills":[],"skills":[]}`,
		`{"version":2,"skills":[{"name":"a","name":"a","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
	} {
		writeInstallFile(t, manifest, body)
		if _, err := readManagedSkillsState(dest); err == nil {
			t.Fatalf("accepted invalid manifest %s", body)
		}
	}
}

func TestListInstallSkillDirsRejectsInvalidSourceName(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "dist", "opencode", "skills")
	mkdirAll(t, filepath.Join(src, "Invalid_Name"))
	if _, err := listInstallSkillDirs(src); err == nil {
		t.Fatal("invalid source skill name accepted")
	}
}

func TestSyncManagedSkillsRecoversMissingAndInterruptedPublication(t *testing.T) {
	root := t.TempDir()
	src, dest := filepath.Join(root, "dist", "opencode", "skills"), filepath.Join(root, "dest")
	writeInstallFile(t, filepath.Join(src, "foundations", "SKILL.md"), "current\n")
	oldRoot := filepath.Join(root, "old")
	writeInstallFile(t, filepath.Join(oldRoot, "SKILL.md"), "old\n")
	oldDigest, err := hashInstallSkillTree(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(dest, loafSkillManifestFile), `{"version":2,"skills":[{"name":"foundations","sha256":"`+oldDigest+`"},{"name":"stale","sha256":"`+oldDigest+`"}]}`)
	// The desired tree was published before a crash, but its old manifest survived.
	writeInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "current\n")
	if err := syncManagedSkillsDirIfExists(src, dest); err != nil {
		t.Fatalf("interrupted publish retry error = %v", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "current\n")
	if _, err := os.Stat(filepath.Join(dest, "stale")); !os.IsNotExist(err) {
		t.Fatalf("missing stale skill stat = %v, want no-op", err)
	}
	if err := os.RemoveAll(filepath.Join(dest, "foundations")); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(dest, "foundations.loaf-backup", "user.txt"), "preserve\n")
	if err := syncManagedSkillsDirIfExists(src, dest); err != nil {
		t.Fatalf("missing desired skill recreate error = %v", err)
	}
	assertInstallFile(t, filepath.Join(dest, "foundations", "SKILL.md"), "current\n")
	assertInstallFile(t, filepath.Join(dest, "foundations.loaf-backup", "user.txt"), "preserve\n")
}

func TestHashInstallSkillTreeRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeInstallFile(t, filepath.Join(root, "SKILL.md"), "skill\n")
	if err := os.Symlink("SKILL.md", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := hashInstallSkillTree(root); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestPublishStagedSkillRestoresExistingDestinationAfterPublishFailure(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "dest-skill")
	backup := filepath.Join(root, "stage", ".backups", "dest-skill")
	writeInstallFile(t, filepath.Join(dest, "SKILL.md"), "existing\n")
	retain, err := publishStagedSkill(filepath.Join(root, "missing-stage"), dest, backup, "", "", true, true)
	if err == nil || retain {
		t.Fatalf("publish missing staged path = retain %v, error %v, want restored error without retention", retain, err)
	}
	assertInstallFile(t, filepath.Join(dest, "SKILL.md"), "existing\n")
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup stat = %v, want restored destination and no backup", err)
	}
}

func TestManagedSkillPublicationAndRetirementRestorePostPreflightMismatch(t *testing.T) {
	root := t.TempDir()
	expected := filepath.Join(root, "expected")
	writeInstallFile(t, filepath.Join(expected, "SKILL.md"), "old\n")
	recorded, err := hashInstallSkillTree(expected)
	if err != nil {
		t.Fatal(err)
	}
	dest, staged, backup := filepath.Join(root, "dest"), filepath.Join(root, "staged"), filepath.Join(root, "backup")
	writeInstallFile(t, filepath.Join(dest, "SKILL.md"), "changed-after-preflight\n")
	writeInstallFile(t, filepath.Join(staged, "SKILL.md"), "desired\n")
	if _, err := publishStagedSkill(staged, dest, backup, recorded, "desired", false, true); err == nil {
		t.Fatal("post-preflight publish mismatch accepted")
	}
	assertInstallFile(t, filepath.Join(dest, "SKILL.md"), "changed-after-preflight\n")
	if _, err := retireManagedSkill(dest, backup+"-retire", recorded, false); err == nil {
		t.Fatal("post-preflight retirement mismatch accepted")
	}
	assertInstallFile(t, filepath.Join(dest, "SKILL.md"), "changed-after-preflight\n")
}

func TestInstallTargetCursorSyncsContentAndRemovesObsoleteHooksOnUpgrade(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "cursor")
	config := filepath.Join(root, ".cursor")
	checkHook := "loaf check --hook check-" + "se" + "crets"
	desired := map[string]any{"command": checkHook, "matcher": "Edit|Write|Bash", loafHookMarker: true}
	writeInstallFile(t, filepath.Join(dist, "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(dist, "agents", "reviewer.md"), "# Reviewer\n")
	writeInstallFile(t, filepath.Join(dist, "templates", "session.md"), "session\n")
	writeInstallFile(t, filepath.Join(dist, "hooks", "post-tool", "check.sh"), "#!/bin/sh\n")
	writeInstallFile(t, filepath.Join(config, "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"user hook"},{"command":"`+checkHook+`","matcher":"Edit|Write|Bash","loaf-managed":true}],"PreToolUse":[{"prompt":"STOP. Before running gh pr merge anything"}]}}`)
	writeInstallFile(t, filepath.Join(config, "commands", "stale.md"), "stale\n")
	writeInstallFile(t, filepath.Join(config, "hooks", "session", "session-start.sh"), "obsolete\n")
	writeTestTargetAdapterManifest(t, dist, "cursor", []map[string]string{
		{"id": "hook-file:hooks/post-tool/check.sh", "kind": "hook-file", "source_path": "hooks/post-tool/check.sh", "destination": "hooks/post-tool/check.sh", "sha256": sha256Hex("#!/bin/sh\n")},
	})
	installTestHookCatalog(t, dist, "cursor", []hookCatalogSource{{
		event: "PostToolUse", hookID: "check-" + "sec" + "rets", typeName: "command", command: checkHook, template: desired,
	}})

	err := installTargetDistribution(targetInstallOptions{
		Target:    "cursor",
		DistDir:   dist,
		ConfigDir: config,
		Upgrade:   true,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
		HookState: installTestHookState(t),
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
		t.Fatalf("PostToolUse hooks = %#v, want user hook plus the converged Loaf entry", postTool)
	}
	if postTool[0]["command"] != "user hook" || postTool[1]["command"] != checkHook {
		t.Fatalf("PostToolUse hooks = %#v, want the user entry preserved in place", postTool)
	}
	// The legacy prompt entry pairs to no identity this version ships, so it is
	// removed; the section the file declared stays declared.
	if preTool, ok := hooks.Hooks["PreToolUse"]; !ok || len(preTool) != 0 {
		t.Fatalf("PreToolUse hooks = %#v, want the retired legacy prompt removed from a preserved section", hooks.Hooks["PreToolUse"])
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetCodexUsesCodexHomeForHooksAndSharedSkillsHome(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex-home")
	dist := filepath.Join(root, "dist", "codex")
	config := filepath.Join(root, "reported-config")
	operations := codexInstallTestOperations(t, root)
	generated := `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`
	writeInstallFile(t, filepath.Join(dist, "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), generated)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"version":1,"description":"user hooks","hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user codex hook"}]}],"Stop":[],"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","if":"Bash(git commit:*)"}]}}`)
	writeTestTargetAdapterManifest(t, dist, "codex", nil)
	installTestHookCatalog(t, dist, "codex", []hookCatalogSource{testCodexHookCatalogSource()})
	// This host has already migrated: the subject here is where the file lives
	// and what gets rendered into it, not what absorption decides.
	hookState := installTestHookStateAfterMigration(t, "codex")

	err := installTargetDistribution(targetInstallOptions{
		Target:              "codex",
		DistDir:             dist,
		ConfigDir:           config,
		Version:             "9.8.7-test.1",
		HomeDir:             home,
		CodexHome:           codexHome,
		CodexRuleOperations: operations,
		HookState:           hookState,
	})
	if err != nil {
		t.Fatalf("install codex error = %v", err)
	}
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	// Unknown top-level fields are the operator's, including the legacy version
	// marker an older Loaf used to strip.
	if hooks.Version != 1 {
		t.Fatalf("codex hooks version = %d, want the file's own top-level field preserved", hooks.Version)
	}
	if hooks.Description != "user hooks" {
		t.Fatalf("codex hooks description = %q, want preserved user description", hooks.Description)
	}
	if len(hooks.Hooks["SessionStart"]) != 2 || hooks.Hooks["SessionStart"][0]["matcher"] != "startup" {
		t.Fatalf("codex hooks = %#v, want nested user matcher group preserved", hooks.Hooks)
	}
	loafGroup := hooks.Hooks["SessionStart"][1]
	loafHandlers, ok := loafGroup["hooks"].([]any)
	if !ok || len(loafHandlers) != 1 {
		t.Fatalf("codex Loaf SessionStart group = %#v, want one command handler", loafGroup)
	}
	loafCommand, ok := loafHandlers[0].(map[string]any)["command"].(string)
	if !ok || strings.Contains(loafCommand, codexJournalExecutablePlaceholder) || !strings.Contains(loafCommand, " journal context --from-hook --codex-hook") || !strings.HasPrefix(loafCommand, "'/") {
		t.Fatalf("codex Loaf command = %#v, want absolute path-pinned command", loafHandlers[0])
	}
	if stop, ok := hooks.Hooks["Stop"]; !ok || len(stop) != 0 {
		t.Fatalf("codex hooks = %#v, want explicitly empty Stop event preserved", hooks.Hooks)
	}
	if postTool, ok := hooks.Hooks["PostToolUse"]; !ok || len(postTool) != 0 {
		t.Fatalf("codex hooks = %#v, want the legacy flat Loaf hook retired", hooks.Hooks)
	}
	if err := installTargetDistribution(targetInstallOptions{
		Target:              "codex",
		DistDir:             dist,
		ConfigDir:           config,
		Version:             "9.8.7-test.1",
		HomeDir:             home,
		CodexHome:           codexHome,
		CodexRuleOperations: operations,
		HookState:           hookState,
	}); err != nil {
		t.Fatalf("second install codex error = %v", err)
	}
	hooks = readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	if len(hooks.Hooks["SessionStart"]) != 2 {
		t.Fatalf("second install Codex SessionStart groups = %#v, want idempotent user plus Loaf groups", hooks.Hooks["SessionStart"])
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetCodexRendersRealGeneratedHookPath(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	var stdout strings.Builder
	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build", "--target", "codex"}); err != nil {
		t.Fatalf("build codex error = %v\n%s", err, stdout.String())
	}
	dist := filepath.Join(root, "dist", "codex")
	generatedBody, err := os.ReadFile(filepath.Join(dist, ".codex", "hooks.json"))
	if err != nil {
		t.Fatalf("read generated Codex hooks error = %v", err)
	}
	generated := string(generatedBody)
	if !strings.Contains(generated, codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix) || strings.Contains(generated, codexJournalHookCommandTemplate) {
		t.Fatalf("generated Codex hooks = %q, want only the install-time executable placeholder", generated)
	}

	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex-home")
	config := filepath.Join(root, "reported-config")
	operations := codexInstallTestOperations(t, root)
	if err := installTargetDistribution(targetInstallOptions{
		Target:              "codex",
		DistDir:             dist,
		ConfigDir:           config,
		Version:             "9.8.7-test.1",
		HomeDir:             home,
		CodexHome:           codexHome,
		CodexRuleOperations: operations,
		ProjectRoot:         root,
		HookState:           installTestHookState(t),
	}); err != nil {
		t.Fatalf("install generated Codex hooks error = %v", err)
	}
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	groups := hooks.Hooks["SessionStart"]
	if len(groups) != 1 {
		t.Fatalf("installed Codex SessionStart groups = %#v, want one generated group", groups)
	}
	handlers, ok := groups[0]["hooks"].([]any)
	if !ok || len(handlers) != 1 {
		t.Fatalf("installed Codex handlers = %#v, want one command handler", groups[0]["hooks"])
	}
	command, ok := handlers[0].(map[string]any)["command"].(string)
	if !ok || strings.Contains(command, codexJournalExecutablePlaceholder) || command == codexJournalHookCommandTemplate || strings.HasPrefix(command, "loaf ") || !strings.HasPrefix(command, "'/") || !strings.HasSuffix(command, codexJournalHookCommandSuffix) {
		t.Fatalf("installed Codex command = %#v, want an absolute path-pinned command without placeholder or PATH bare loaf", handlers[0])
	}
	if runtime.GOOS != "windows" {
		if _, ok := handlers[0].(map[string]any)["commandWindows"]; ok {
			t.Fatalf("installed Codex command handler = %#v, want platform-inapplicable commandWindows omitted on %s", handlers[0], runtime.GOOS)
		}
	}
}

func TestCodexMatcherValidationAcceptsOptionalNullsAndEmptyCommand(t *testing.T) {
	hook, err := decodeCodexHookObject(json.RawMessage(`{"matcher":null,"hooks":[{"type":"command","command":"   ","commandWindows":null,"timeout":null,"statusMessage":null}]}`))
	if err != nil {
		t.Fatalf("decode Codex matcher group error = %v", err)
	}
	if !isValidCodexMatcherGroup(hook) {
		t.Fatalf("Codex matcher group = %#v, want current-schema optional nulls and empty command accepted", hook)
	}

	for name, raw := range map[string]json.RawMessage{
		"empty group":  json.RawMessage(`{}`),
		"null matcher": json.RawMessage(`{"matcher":null}`),
	} {
		t.Run(name, func(t *testing.T) {
			hook, err := decodeCodexHookObject(raw)
			if err != nil {
				t.Fatalf("decode Codex matcher group error = %v", err)
			}
			if !isValidCodexMatcherGroup(hook) {
				t.Fatalf("Codex matcher group = %#v, want valid empty group", hook)
			}
		})
	}
}

// A top-level field whose value is null is still the operator's. The reader
// keeps it as the raw value it was written as, so republishing the document
// hands it back unchanged rather than dropping or normalizing it.
func TestReadHookFilePreservesNullTopLevelFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	body := `{"description":null,"hooks":{}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write hooks file: %v", err)
	}
	file, err := readHookFile(path)
	if err != nil {
		t.Fatalf("readHookFile error = %v", err)
	}
	if got := string(file.fields["description"]); got != "null" {
		t.Fatalf("description = %s, want null preserved", got)
	}
	republished, err := file.marshal()
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(republished), `"description": null`) {
		t.Fatalf("republished = %s, want the null description carried through", republished)
	}
}

func TestCodexHookUint64RejectsLossyFloatValues(t *testing.T) {
	for _, value := range []any{float64(-1), float64(1.5), float64(1<<53 + 2), math.MaxFloat64} {
		if _, ok := codexHookUint64(value); ok {
			t.Errorf("codexHookUint64(%v) accepted lossy or invalid float", value)
		}
	}
	for value, want := range map[any]uint64{float64(0): 0, float64(1 << 53): 1 << 53} {
		got, ok := codexHookUint64(value)
		if !ok || got != want {
			t.Errorf("codexHookUint64(%v) = %d, %v, want %d, true", value, got, ok, want)
		}
	}
	if got, ok := codexHookUint64(json.Number("18446744073709551615")); !ok || got != ^uint64(0) {
		t.Fatalf("codexHookUint64(max uint64) = %d, %v, want exact max uint64", got, ok)
	}
}

func TestCodexHookExecutableRenderingUsesLiteralCanonicalShellQuote(t *testing.T) {
	path := "/trusted/Loaf $release/o'brien/loaf"
	raw := json.RawMessage(`{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}`)
	rendered, err := renderCodexHookExecutable(raw, path)
	if err != nil {
		t.Fatalf("renderCodexHookExecutable error = %v", err)
	}
	var hook map[string]any
	if err := json.Unmarshal(rendered, &hook); err != nil {
		t.Fatal(err)
	}
	handlers := hook["hooks"].([]any)
	command := handlers[0].(map[string]any)["command"].(string)
	want := journalContextShellQuote(path) + codexJournalHookCommandSuffix
	if command != want || !isExactCodexJournalHookCommand(command) {
		t.Fatalf("rendered command = %q, want canonical literal %q", command, want)
	}
	if strings.Contains(command, codexJournalExecutablePlaceholder) {
		t.Fatalf("rendered command retained placeholder: %q", command)
	}
}

func TestCodexWindowsHookExecutableRenderingUsesCmdOuterQuote(t *testing.T) {
	path := `C:\Program Files (x86)\Loaf & Co\loaf.exe`
	raw := json.RawMessage(`{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook","commandWindows":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}`)
	rendered, err := renderCodexHookExecutableForOS(raw, path, "windows")
	if err != nil {
		t.Fatalf("renderCodexHookExecutableForOS error = %v", err)
	}
	var hook map[string]any
	if err := json.Unmarshal(rendered, &hook); err != nil {
		t.Fatal(err)
	}
	handler := hook["hooks"].([]any)[0].(map[string]any)
	windowsCommand, ok := handler["commandWindows"].(string)
	if !ok {
		t.Fatalf("rendered handler = %#v, want commandWindows", handler)
	}
	want := `""C:\Program Files (x86)\Loaf & Co\loaf.exe" journal context --from-hook --codex-hook"`
	if windowsCommand != want || !isExactCodexJournalHookCommandWindows(windowsCommand) {
		t.Fatalf("rendered commandWindows = %q, want canonical cmd.exe command %q", windowsCommand, want)
	}
	if handler["command"] != want || handler["command"] != windowsCommand {
		t.Fatalf("rendered command = %#v, want same canonical cmd.exe command as commandWindows", handler["command"])
	}
	rotatedPath := `C:\Loaf\v2\loaf.exe`
	rotated, err := renderCodexHookExecutableForOS(raw, rotatedPath, "windows")
	if err != nil {
		t.Fatalf("render rotated Codex hook error = %v", err)
	}
	var rotatedHook map[string]any
	if err := json.Unmarshal(rotated, &rotatedHook); err != nil {
		t.Fatal(err)
	}
	rotatedHandler := rotatedHook["hooks"].([]any)[0].(map[string]any)
	rotatedWant := `""C:\Loaf\v2\loaf.exe" journal context --from-hook --codex-hook"`
	if rotatedHandler["command"] != rotatedWant || rotatedHandler["commandWindows"] != rotatedWant {
		t.Fatalf("rotated handler = %#v, want both command fields updated atomically", rotatedHandler)
	}
	ownedHook := map[string]any{"matcher": codexJournalHookMatcher, "hooks": []any{handler}}
	if owned, conflict := codexHookOwnershipForOS(ownedHook, "windows"); !owned || conflict {
		t.Fatalf("Windows owned hook = %#v, want exact three-key ownership", ownedHook)
	}
	for name, mutate := range map[string]func(map[string]any){
		"changed command": func(h map[string]any) {
			h["hooks"].([]any)[0].(map[string]any)["command"] = want + " altered"
		},
		"changed commandWindows": func(h map[string]any) {
			h["hooks"].([]any)[0].(map[string]any)["commandWindows"] = want + " altered"
		},
		"removed commandWindows": func(h map[string]any) {
			delete(h["hooks"].([]any)[0].(map[string]any), "commandWindows")
		},
	} {
		t.Run(name, func(t *testing.T) {
			modified := map[string]any{"matcher": codexJournalHookMatcher, "hooks": []any{map[string]any{"type": "command", "command": want, "commandWindows": want}}}
			mutate(modified)
			if owned, conflict := codexHookOwnershipForOS(modified, "windows"); owned || !conflict {
				t.Fatalf("modified Windows hook = %#v, want ownership conflict", modified)
			}
		})
	}

	for _, invalid := range []string{
		"loaf.exe",
		`C:/Program Files/Loaf/loaf.exe`,
		`C:\Program Files\Loaf%20\loaf.exe`,
		`C:\Program Files\Loaf!\loaf.exe`,
		`C:\Program Files\Loaf"\loaf.exe`,
		"C:\\Program Files\\Loaf\n\\loaf.exe",
		`\\.\pipe\loaf.exe`,
		`\\?\C:\Program Files\Loaf\loaf.exe`,
	} {
		if _, err := codexWindowsJournalContextCommand(invalid); err == nil {
			t.Errorf("codexWindowsJournalContextCommand(%q) succeeded, want canonical-path rejection", invalid)
		}
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
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf-modes.ts"), "export const modes = true;\n")
	writeTestTargetAdapterManifest(t, ampDist, "amp", []map[string]string{
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("export default function () {}\n")},
		{"id": "plugin:.amp/plugins/loaf-modes.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf-modes.ts", "destination": "plugins/loaf-modes.ts", "sha256": sha256Hex("export const modes = true;\n")},
	})

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
	assertInstallFile(t, filepath.Join(ampPlugins, "loaf-modes.ts"), "export const modes = true;\n")
	assertInstallFile(t, filepath.Join(ampConfig, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetAmpDefaultsPluginToConfigPluginsDir(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	ampDist := filepath.Join(root, "dist", "amp")
	ampConfig := filepath.Join(root, "xdg", "amp")
	legacyPlugins := filepath.Join(home, ".amp", "plugins")
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf.ts"), "export default function () {}\n")
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf-modes.ts"), "export const modes = true;\n")
	writeTestTargetAdapterManifest(t, ampDist, "amp", []map[string]string{
		{"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("export default function () {}\n")},
		{"id": "plugin:.amp/plugins/loaf-modes.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf-modes.ts", "destination": "plugins/loaf-modes.ts", "sha256": sha256Hex("export const modes = true;\n")},
	})

	if err := installTargetDistribution(targetInstallOptions{
		Target:    "amp",
		DistDir:   ampDist,
		ConfigDir: ampConfig,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
	}); err != nil {
		t.Fatalf("install amp error = %v", err)
	}
	assertInstallFile(t, filepath.Join(ampConfig, "plugins", "loaf.ts"), "export default function () {}\n")
	assertInstallFile(t, filepath.Join(ampConfig, "plugins", "loaf-modes.ts"), "export const modes = true;\n")
	assertInstallPathMissing(t, filepath.Join(legacyPlugins, "loaf.ts"))
	assertInstallPathMissing(t, filepath.Join(legacyPlugins, "loaf-modes.ts"))
	assertInstallFile(t, filepath.Join(ampConfig, loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "loaf", "install-targets", "amp.json"), strings.Join([]string{
		"{",
		"  \"version\": \"9.8.7-test.1\",",
		"  \"target\": \"amp\",",
		"  \"config_dir\": \"" + ampConfig + "\",",
		"  \"skills_dir\": \"" + filepath.Join(home, ".agents", "skills") + "\"",
		"}",
	}, "\n")+"\n")
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

func testAmpModesPredecessor(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(testRepositoryRoot(t), "internal", "cli", "testdata", "amp", "loaf-modes-predecessor.ts"))
	if err != nil {
		t.Fatalf("ReadFile(amp modes predecessor) error = %v", err)
	}
	return string(body)
}

func findArtifactPlanDecision(t *testing.T, decisions []artifactPlanDecision, id string) artifactPlanDecision {
	t.Helper()
	for _, decision := range decisions {
		if decision.ID == id {
			return decision
		}
	}
	t.Fatalf("plan decisions = %#v, want %s", decisions, id)
	return artifactPlanDecision{}
}

func writeTestTargetAdapterManifest(t *testing.T, dist string, target string, artifacts []map[string]string) {
	t.Helper()
	allArtifacts := []map[string]any{{
		"id":          "managed-instructions",
		"kind":        "instruction",
		"destination": "project-instructions",
		"sha256":      fencedContentFingerprint(generateFencedContent()),
	}}
	for _, artifact := range artifacts {
		entry := make(map[string]any, len(artifact)+1)
		for key, value := range artifact {
			entry[key] = value
		}
		if artifact["kind"] == "hook-file" || artifact["kind"] == "plugin" {
			info, err := os.Lstat(filepath.Join(dist, filepath.FromSlash(artifact["source_path"])))
			if err != nil {
				t.Fatalf("Lstat target adapter source error = %v", err)
			}
			entry["mode"] = uint32(info.Mode().Perm())
		}
		allArtifacts = append(allArtifacts, entry)
	}
	sort.Slice(allArtifacts, func(i, j int) bool { return allArtifacts[i]["id"].(string) < allArtifacts[j]["id"].(string) })
	body, err := json.MarshalIndent(map[string]any{
		"version":                     1,
		"target":                      target,
		"package_version":             "9.8.7-test.1",
		"capability_contract_version": 3,
		"adapters":                    []string{target + "-test-adapter-v1"},
		"artifacts":                   allArtifacts,
	}, "", "  ")
	if err != nil {
		t.Fatalf("Marshal target adapter manifest error = %v", err)
	}
	writeInstallFile(t, filepath.Join(dist, ".loaf-target-manifest.json"), string(body)+"\n")
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

func assertInstallMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode(%s) = %#o, want %#o", path, got, want)
	}
}

func assertInstallPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing path", path, err)
	}
}

// loafHookMarker is the human-legible provenance a Cursor entry carries.
// Recognition deliberately does not depend on it — Decision 6 makes ownership a
// property of the command, not of a field anyone can write — so it survives as
// a test-side name for the field the builder emits and assertions read.
const loafHookMarker = "loaf-managed"

// installedHooksFile is the shape tests decode a hooks file into. Production
// reads these files through readHookFile, which keeps every foreign value raw;
// a test that only wants to look at what landed can afford the lossy decode.
type installedHooksFile struct {
	Version     int                         `json:"version,omitempty"`
	Description string                      `json:"description,omitempty"`
	Hooks       map[string][]map[string]any `json:"hooks"`
}

func readInstallHooks(t *testing.T, path string) installedHooksFile {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var hooks installedHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v\n%s", path, err, body)
	}
	return hooks
}
