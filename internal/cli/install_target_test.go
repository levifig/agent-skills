package cli

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
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
	operations := codexInstallTestOperations(t, root)
	writeInstallFile(t, filepath.Join(dist, "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"version":1,"description":"user hooks","hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user codex hook"}]}],"Stop":[],"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","if":"Bash(git commit:*)"}]}}`)

	err := installTargetDistribution(targetInstallOptions{
		Target:              "codex",
		DistDir:             dist,
		ConfigDir:           config,
		Version:             "9.8.7-test.1",
		HomeDir:             home,
		CodexHome:           codexHome,
		CodexRuleOperations: operations,
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
	if _, ok := hooks.Hooks["PostToolUse"]; ok {
		t.Fatalf("codex hooks = %#v, want legacy flat Loaf hook retired", hooks.Hooks)
	}
	if err := installTargetDistribution(targetInstallOptions{
		Target:              "codex",
		DistDir:             dist,
		ConfigDir:           config,
		Version:             "9.8.7-test.1",
		HomeDir:             home,
		CodexHome:           codexHome,
		CodexRuleOperations: operations,
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

func TestInstallTargetCodexPreservesPromptAndAgentHandlers(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex-home")
	dist := filepath.Join(root, "dist", "codex")
	config := filepath.Join(root, "reported-config")
	operations := codexInstallTestOperations(t, root)
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"SessionStart":[{}, {"matcher":null}, {"matcher":"resume","hooks":[{"type":"prompt"}]},{"matcher":"clear","hooks":[{"type":"agent"}]},{"matcher":"compact","hooks":[{"type":"command","command":"user hook","command_windows":"powershell user hook","timeout":0,"async":true,"statusMessage":"checking"}]}]}}`)

	if err := installTargetDistribution(targetInstallOptions{Target: "codex", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home, CodexHome: codexHome, CodexRuleOperations: operations}); err != nil {
		t.Fatalf("install codex error = %v", err)
	}
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	groups := hooks.Hooks["SessionStart"]
	if len(groups) != 6 {
		t.Fatalf("Codex SessionStart groups = %#v, want five user groups plus Loaf", groups)
	}
	for index, wantType := range []string{"prompt", "agent", "command"} {
		groupIndex := index + 2
		handlers, ok := groups[groupIndex]["hooks"].([]any)
		if !ok || len(handlers) != 1 {
			t.Fatalf("Codex user group %d handlers = %#v, want one handler", groupIndex, groups[groupIndex]["hooks"])
		}
		handler, ok := handlers[0].(map[string]any)
		if !ok || handler["type"] != wantType {
			t.Fatalf("Codex user group %d handler = %#v, want type %q", groupIndex, handler, wantType)
		}
	}
	command := groups[4]["hooks"].([]any)[0].(map[string]any)
	if command["command_windows"] != "powershell user hook" || command["timeout"] != float64(0) || command["async"] != true || command["statusMessage"] != "checking" {
		t.Fatalf("Codex command handler = %#v, want valid current-schema fields preserved", command)
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
		"empty group":      json.RawMessage(`{}`),
		"null matcher":     json.RawMessage(`{"matcher":null}`),
		"null description": json.RawMessage(`{"description":null,"hooks":{}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "null description" {
				path := filepath.Join(t.TempDir(), "hooks.json")
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatalf("write Codex hooks file: %v", err)
				}
				loaded, err := loadCodexHooksRawFileStrict(path)
				if err != nil {
					t.Fatalf("load Codex hooks file error = %v", err)
				}
				if string(loaded.Description) != "null" {
					t.Fatalf("loaded description = %s, want null preserved", loaded.Description)
				}
				return
			}
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
		{name: "prompt-extra-field", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"prompt","unexpected":true}]}]}}`, want: "unsupported matcher entry"},
		{name: "agent-extra-field", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"agent","unexpected":true}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-handler-field", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","bogus":true}]}]}}`, want: "unsupported matcher entry"},
		{name: "wrong-handler-field-type", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","async":"true"}]}]}}`, want: "unsupported matcher entry"},
		{name: "duplicate-command-windows-alias", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","commandWindows":"windows user","command_windows":"windows alias"}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-group-field", body: `{"hooks":{"SessionStart":[{"event":"startup","hooks":[{"type":"command","command":"echo user"}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-event", body: `{"hooks":{"BogusEvent":[{"hooks":[{"type":"command","command":"echo user"}]}]}}`, want: "unsupported hook event"},
		{name: "null-event-array", body: `{"hooks":{"Stop":null}}`, want: "must be an array"},
		{name: "timeout-negative", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":-1}]}]}}`, want: "unsupported matcher entry"},
		{name: "timeout-fractional", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":1.5}]}]}}`, want: "unsupported matcher entry"},
		{name: "timeout-too-large", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":18446744073709551616}]}]}}`, want: "unsupported matcher entry"},
		{name: "timeout-wrong-type", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":"1"}]}]}}`, want: "unsupported matcher entry"},
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
			operations := codexInstallTestOperations(t, root)
			writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{}}`)
			writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), tc.body)

			err := installTargetDistribution(targetInstallOptions{
				Target:              "codex",
				DistDir:             dist,
				ConfigDir:           config,
				Version:             "9.8.7-test.1",
				HomeDir:             home,
				CodexHome:           codexHome,
				CodexRuleOperations: operations,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("install codex error = %v, want %q", err, tc.want)
			}
			assertInstallFile(t, filepath.Join(codexHome, "hooks.json"), tc.body)
		})
	}
}

func TestInstallTargetCodexRejectsModifiedOwnedGroupAndPlaceholderLeak(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex-home")
	dist := filepath.Join(root, "dist", "codex")
	config := filepath.Join(root, "reported-config")
	operations := codexInstallTestOperations(t, root)
	generated := `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), generated)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"'/usr/local/bin/loaf' journal context --from-hook --codex-hook"},{"type":"command","command":"user hook"}]}]}}`)
	err := installTargetDistribution(targetInstallOptions{Target: "codex", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home, CodexHome: codexHome, CodexRuleOperations: operations})
	if err == nil || !strings.Contains(err.Error(), "modified Loaf SessionStart matcher group") {
		t.Fatalf("modified owned group error = %v, want ownership conflict", err)
	}
	assertInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"'/usr/local/bin/loaf' journal context --from-hook --codex-hook"},{"type":"command","command":"user hook"}]}]}}`)

	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{}}`)
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook","statusMessage":"{{LOAF_EXECUTABLE}}"}]}]}}`)
	err = installTargetDistribution(targetInstallOptions{Target: "codex", DistDir: dist, ConfigDir: config, Version: "9.8.7-test.1", HomeDir: home, CodexHome: codexHome, CodexRuleOperations: operations})
	if err == nil || !strings.Contains(err.Error(), "placeholder remains") {
		t.Fatalf("placeholder leak error = %v, want strict rejection", err)
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

func TestMergeCodexWindowsHookIsIdempotentAndConflictSafe(t *testing.T) {
	root := realpath(t, t.TempDir())
	destPath := filepath.Join(root, "codex-home", "hooks.json")
	loafPath := filepath.Join(root, "dist", "codex", ".codex", "hooks.json")
	writeInstallFile(t, loafPath, `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook","commandWindows":"{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"}]}]}}`)
	writeInstallFile(t, destPath, `{"hooks":{}}`+"\n")
	pathV1 := `C:\Program Files (x86)\Loaf\v1\loaf.exe`
	if err := mergeCodexHookFilesForOSWithExecutable(destPath, loafPath, root, nil, "windows", pathV1); err != nil {
		t.Fatalf("first Windows merge error = %v", err)
	}
	firstBody, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read first Windows merge = %v", err)
	}
	assertCodexWindowsInstalledGroup(t, firstBody, pathV1)
	if err := mergeCodexHookFilesForOSWithExecutable(destPath, loafPath, root, nil, "windows", pathV1); err != nil {
		t.Fatalf("second Windows merge error = %v", err)
	}
	secondBody, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read second Windows merge = %v", err)
	}
	if string(secondBody) != string(firstBody) {
		t.Fatalf("second Windows merge changed exact managed group:\nfirst=%s\nsecond=%s", firstBody, secondBody)
	}

	pathV2 := `C:\Loaf\v2\loaf.exe`
	if err := mergeCodexHookFilesForOSWithExecutable(destPath, loafPath, root, nil, "windows", pathV2); err != nil {
		t.Fatalf("rotated Windows merge error = %v", err)
	}
	rotatedBody, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read rotated Windows merge = %v", err)
	}
	assertCodexWindowsInstalledGroup(t, rotatedBody, pathV2)

	for name, mutate := range map[string]func(map[string]any){
		"changed command": func(handler map[string]any) {
			handler["command"] = handler["command"].(string) + " altered"
		},
		"changed commandWindows": func(handler map[string]any) {
			handler["commandWindows"] = handler["commandWindows"].(string) + " altered"
		},
		"removed commandWindows": func(handler map[string]any) {
			delete(handler, "commandWindows")
		},
	} {
		t.Run(name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal(rotatedBody, &raw); err != nil {
				t.Fatalf("decode rotated Windows hooks = %v", err)
			}
			groups := raw["hooks"].(map[string]any)["SessionStart"].([]any)
			handler := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
			mutate(handler)
			body, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("encode conflicting Windows hooks = %v", err)
			}
			writeInstallFile(t, destPath, string(body))
			if err := mergeCodexHookFilesForOSWithExecutable(destPath, loafPath, root, nil, "windows", pathV2); err == nil || !strings.Contains(err.Error(), "modified Loaf SessionStart matcher group") {
				t.Fatalf("conflicting Windows merge error = %v, want ownership conflict", err)
			}
		})
	}
}

func assertCodexWindowsInstalledGroup(t *testing.T, body []byte, executable string) {
	t.Helper()
	var hooks codexHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		t.Fatalf("decode Windows hooks = %v", err)
	}
	groups := hooks.Hooks["SessionStart"]
	if len(groups) != 1 {
		t.Fatalf("Windows groups = %#v, want one managed group", groups)
	}
	handlers := groups[0]["hooks"].([]any)
	if len(handlers) != 1 {
		t.Fatalf("Windows handlers = %#v, want one managed handler", groups[0]["hooks"])
	}
	handler := handlers[0].(map[string]any)
	want := `""` + executable + `"` + codexJournalHookCommandSuffix + `"`
	if handler["command"] != want || handler["commandWindows"] != want {
		t.Fatalf("Windows managed handler = %#v, want equal command fields %q", handler, want)
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

func TestInstallTargetAmpDefaultsPluginToConfigPluginsDir(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	ampDist := filepath.Join(root, "dist", "amp")
	ampConfig := filepath.Join(root, "xdg", "amp")
	legacyPlugins := filepath.Join(home, ".amp", "plugins")
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf.ts"), "export default function () {}\n")

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
	assertInstallPathMissing(t, filepath.Join(legacyPlugins, "loaf.ts"))
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
