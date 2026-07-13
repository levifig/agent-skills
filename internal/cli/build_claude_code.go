package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const nativeClaudePluginName = "loaf"
const nativeClaudePluginDescription = "Loaf - An Opinionated Agentic Framework"
const nativeClaudeRepository = "https://github.com/levifig/loaf"

type nativeClaudeMarketplaceJSON struct {
	Name     string                      `json:"name"`
	Owner    nativeClaudeOwnerJSON       `json:"owner"`
	Metadata nativeClaudeMetadataJSON    `json:"metadata"`
	Plugins  []nativeClaudePluginRefJSON `json:"plugins"`
}

type nativeClaudeOwnerJSON struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type nativeClaudeMetadataJSON struct {
	Description string `json:"description"`
	Version     string `json:"version"`
}

type nativeClaudePluginRefJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Version     string `json:"version"`
	License     string `json:"license"`
	Repository  string `json:"repository"`
}

type nativeClaudePluginJSON struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Repository  string `json:"repository"`
	License     string `json:"license"`
}

type nativeClaudeHooksJSON struct {
	Hooks nativeClaudeHookGroupsJSON `json:"hooks"`
}

type nativeClaudeHookGroupsJSON struct {
	PreToolUse       []nativeClaudeMatcherHooksJSON `json:"PreToolUse,omitempty"`
	PostToolUse      []nativeClaudeMatcherHooksJSON `json:"PostToolUse,omitempty"`
	SessionStart     []nativeClaudeSessionHooksJSON `json:"SessionStart,omitempty"`
	SessionEnd       []nativeClaudeSessionHooksJSON `json:"SessionEnd,omitempty"`
	TaskCompleted    []nativeClaudeSessionHooksJSON `json:"TaskCompleted,omitempty"`
	UserPromptSubmit []nativeClaudeSessionHooksJSON `json:"UserPromptSubmit,omitempty"`
	PreCompact       []nativeClaudeSessionHooksJSON `json:"PreCompact,omitempty"`
	PostCompact      []nativeClaudeSessionHooksJSON `json:"PostCompact,omitempty"`
	Stop             []nativeClaudeSessionHooksJSON `json:"Stop,omitempty"`
}

type nativeClaudeMatcherHooksJSON struct {
	Matcher string `json:"matcher"`
	Hooks   []any  `json:"hooks"`
}

type nativeClaudeSessionHooksJSON struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []any  `json:"hooks"`
}

type nativeClaudePreCommandHookJSON struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	If          string `json:"if,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
	FailClosed  bool   `json:"failClosed,omitempty"`
}

type nativeClaudePostCommandHookJSON struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	If          string `json:"if,omitempty"`
	Description string `json:"description,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
	FailClosed  bool   `json:"failClosed,omitempty"`
}

type nativeClaudePrePromptHookJSON struct {
	Type    string `json:"type"`
	Prompt  string `json:"prompt"`
	If      string `json:"if,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type nativeClaudePostPromptHookJSON struct {
	Type        string `json:"type"`
	Prompt      string `json:"prompt"`
	If          string `json:"if,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
}

type nativeClaudeSessionCommandHookJSON struct {
	Type        string `json:"type"`
	Timeout     int    `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
	If          string `json:"if,omitempty"`
	Command     string `json:"command"`
	FailClosed  bool   `json:"failClosed,omitempty"`
}

type nativeClaudeSessionPromptHookJSON struct {
	Type        string `json:"type"`
	Timeout     int    `json:"timeout,omitempty"`
	Description string `json:"description,omitempty"`
	If          string `json:"if,omitempty"`
	Prompt      string `json:"prompt"`
}

func runNativeBuildClaudeCode(root string, out io.Writer) error {
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	targetStart := time.Now()
	fmt.Fprintf(out, "  %s claude-code...", ansiCyan("building"))
	if err := buildNativeClaudeCodeTarget(root); err != nil {
		fmt.Fprintf(out, "\r  %s claude-code\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s claude-code %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(targetStart)+")"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func buildNativeClaudeCodeTarget(root string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	pluginsDir := filepath.Join(root, "plugins")
	marketplaceDir := filepath.Join(root, ".claude-plugin")
	if err := os.RemoveAll(pluginsDir); err != nil {
		return err
	}
	if err := os.RemoveAll(marketplaceDir); err != nil {
		return err
	}
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(marketplaceDir, 0o755); err != nil {
		return err
	}
	if err := writeNativeClaudeMarketplace(marketplaceDir, version); err != nil {
		return err
	}

	srcDir := filepath.Join(root, "content")
	pluginDir := filepath.Join(pluginsDir, nativeClaudePluginName)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}
	hooks, err := readNativeBuildHooks(filepath.Join(root, "config", "hooks.yaml"))
	if err != nil {
		return err
	}
	if err := writeNativeClaudePluginFiles(pluginDir, hooks, version); err != nil {
		return err
	}
	if err := copyNativeBuildAgents(srcDir, filepath.Join(pluginDir, "agents"), "claude-code", version, nil, true); err != nil {
		return err
	}

	knownCommands, err := nativeClaudeKnownCommands(root)
	if err != nil {
		return err
	}
	transformMd := func(content string) string {
		return nativeClaudeScopeCommands(substituteNativeBuildHarnessLanguage(content, "claude-code"), knownCommands)
	}
	if err := copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        filepath.Join(root, "dist"),
		destDir:       filepath.Join(pluginDir, "skills"),
		sidecarSrcDir: srcDir,
		targetName:    "claude-code",
		version:       version,
		targetsConfig: targetsConfig,
		transformMd:   transformMd,
	}); err != nil {
		return err
	}
	if err := copyNativeClaudeHooks(hooks, srcDir, pluginDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, ".lsp.json"), []byte(nativeClaudeLSPJSON()), 0o644); err != nil {
		return err
	}
	if pathExistsNative(filepath.Join(srcDir, "SETUP.md")) {
		if err := copyNativeBuildFile(filepath.Join(srcDir, "SETUP.md"), filepath.Join(pluginDir, "SETUP.md")); err != nil {
			return err
		}
	}
	if err := copyNativeClaudeRuntimeFiles(root, pluginDir, version); err != nil {
		return err
	}
	return nil
}

func writeNativeClaudeMarketplace(dir string, version string) error {
	payload := nativeClaudeMarketplaceJSON{
		Name: "levifig-loaf",
		Owner: nativeClaudeOwnerJSON{
			Name:  "Levi Figueira",
			Email: "me@levifig.com",
		},
		Metadata: nativeClaudeMetadataJSON{
			Description: nativeClaudePluginDescription,
			Version:     version,
		},
		Plugins: []nativeClaudePluginRefJSON{{
			Name:        nativeClaudePluginName,
			Description: nativeClaudePluginDescription,
			Source:      "./plugins/" + nativeClaudePluginName,
			Version:     version,
			License:     "MIT",
			Repository:  nativeClaudeRepository,
		}},
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "marketplace.json"), body, 0o644)
}

func writeNativeClaudePluginFiles(pluginDir string, hooks []nativeBuildHook, version string) error {
	hooksDir := filepath.Join(pluginDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	hooksBody, err := json.MarshalIndent(nativeClaudeHooksPayload(hooks), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.json"), hooksBody, 0o644); err != nil {
		return err
	}

	pluginJSONDir := filepath.Join(pluginDir, ".claude-plugin")
	if err := os.MkdirAll(pluginJSONDir, 0o755); err != nil {
		return err
	}
	pluginBody, err := json.MarshalIndent(nativeClaudePluginJSON{
		Name:        nativeClaudePluginName,
		Version:     version,
		Description: nativeClaudePluginDescription,
		Repository:  nativeClaudeRepository,
		License:     "MIT",
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pluginJSONDir, "plugin.json"), pluginBody, 0o644)
}

func nativeClaudeHooksPayload(hooks []nativeBuildHook) nativeClaudeHooksJSON {
	var payload nativeClaudeHooksJSON
	payload.Hooks.PreToolUse = nativeClaudeMatcherGroups(filterNativeBuildHooksBySection(hooks, "pre-tool"), true)
	payload.Hooks.PostToolUse = nativeClaudeMatcherGroups(filterNativeBuildHooksBySection(hooks, "post-tool"), false)
	for _, hook := range filterNativeBuildHooksBySection(hooks, "session") {
		// Claude receives continuity on SessionStart for every native source,
		// including compact. The generic PostCompact declaration remains in
		// config/hooks.yaml for targets that support it, but Claude must not
		// render a second continuity handler there.
		if hook.id == "post-compact" && hook.event == "PostCompact" {
			continue
		}
		if hook.id == "session-start-loaf" && hook.event == "SessionStart" {
			hook.command = "loaf journal context --from-hook --claude-code"
		}
		group := nativeClaudeSessionHooksJSON{Hooks: []any{nativeClaudeSessionHookEntry(hook)}}
		if hook.id == "session-start-loaf" && hook.event == "SessionStart" {
			group.Matcher = "startup|resume|clear|compact"
		}
		switch hook.event {
		case "SessionStart":
			payload.Hooks.SessionStart = append(payload.Hooks.SessionStart, group)
		case "SessionEnd":
			payload.Hooks.SessionEnd = append(payload.Hooks.SessionEnd, group)
		case "TaskCompleted":
			payload.Hooks.TaskCompleted = append(payload.Hooks.TaskCompleted, group)
		case "UserPromptSubmit":
			payload.Hooks.UserPromptSubmit = append(payload.Hooks.UserPromptSubmit, group)
		case "PreCompact":
			payload.Hooks.PreCompact = append(payload.Hooks.PreCompact, group)
		case "PostCompact":
			payload.Hooks.PostCompact = append(payload.Hooks.PostCompact, group)
		case "Stop":
			payload.Hooks.Stop = append(payload.Hooks.Stop, group)
		}
	}
	return payload
}

func filterNativeBuildHooksBySection(hooks []nativeBuildHook, section string) []nativeBuildHook {
	var filtered []nativeBuildHook
	for _, hook := range hooks {
		if hook.section == section {
			filtered = append(filtered, hook)
		}
	}
	return filtered
}

func nativeClaudeMatcherGroups(hooks []nativeBuildHook, preTool bool) []nativeClaudeMatcherHooksJSON {
	var groups []nativeClaudeMatcherHooksJSON
	for _, hook := range hooks {
		matcher := hook.matcher
		if matcher == "" {
			matcher = "Edit|Write"
		}
		entry := nativeClaudeToolHookEntry(hook, preTool)
		found := false
		for i := range groups {
			if groups[i].Matcher == matcher {
				groups[i].Hooks = append(groups[i].Hooks, entry)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, nativeClaudeMatcherHooksJSON{
				Matcher: matcher,
				Hooks:   []any{entry},
			})
		}
	}
	return groups
}

func nativeClaudeToolHookEntry(hook nativeBuildHook, preTool bool) any {
	if hook.typeName == "prompt" {
		if preTool {
			return nativeClaudePrePromptHookJSON{
				Type:    "prompt",
				Prompt:  hook.prompt,
				If:      hook.ifCondition,
				Timeout: nativeClaudeOptionalSecondsTimeout(hook.timeout),
			}
		}
		return nativeClaudePostPromptHookJSON{
			Type:        "prompt",
			Prompt:      hook.prompt,
			If:          hook.ifCondition,
			Timeout:     nativeClaudeOptionalSecondsTimeout(hook.timeout),
			Description: hook.description,
		}
	}
	if preTool {
		return nativeClaudePreCommandHookJSON{
			Type:        "command",
			Command:     nativeClaudeHookCommand(hook),
			If:          hook.ifCondition,
			Timeout:     nativeClaudeOptionalSecondsTimeout(hook.timeout),
			Description: hook.description,
			FailClosed:  hook.failClosed,
		}
	}
	return nativeClaudePostCommandHookJSON{
		Type:        "command",
		Command:     nativeClaudeHookCommand(hook),
		If:          hook.ifCondition,
		Description: hook.description,
		Timeout:     nativeClaudeOptionalSecondsTimeout(hook.timeout),
		FailClosed:  hook.failClosed,
	}
}

func nativeClaudeSessionHookEntry(hook nativeBuildHook) any {
	if hook.typeName == "prompt" {
		return nativeClaudeSessionPromptHookJSON{
			Type:        "prompt",
			Timeout:     nativeClaudeOptionalSecondsTimeout(hook.timeout),
			Description: hook.description,
			If:          hook.ifCondition,
			Prompt:      hook.prompt,
		}
	}
	return nativeClaudeSessionCommandHookJSON{
		Type:        firstNonEmptyNativeClaude(hook.typeName, "command"),
		Timeout:     nativeClaudeOptionalSecondsTimeout(hook.timeout),
		Description: hook.description,
		If:          hook.ifCondition,
		Command:     nativeClaudeHookCommand(hook),
		FailClosed:  hook.failClosed,
	}
}

func nativeClaudeOptionalSecondsTimeout(timeout int) int {
	if timeout == 0 {
		return 0
	}
	return timeout / 1000
}

func nativeClaudeHookCommand(hook nativeBuildHook) string {
	if hook.instruction != "" {
		return `cat "${CLAUDE_PLUGIN_ROOT}/hooks/` + hook.instruction + `"`
	}
	if nativeClaudeBinaryPathHooks[hook.id] {
		if hook.command == "" {
			return `"${CLAUDE_PLUGIN_ROOT}/bin/loaf" check --hook ` + hook.id + nativeCheckAdvisorySuffix(hook)
		}
		return strings.ReplaceAll(hook.command, "loaf", `"${CLAUDE_PLUGIN_ROOT}/bin/loaf"`)
	}
	if hook.command != "" {
		return strings.ReplaceAll(hook.command, "${CLAUDE_PLUGIN_ROOT}", "${CLAUDE_PLUGIN_ROOT}")
	}
	filename := filepath.Base(hook.script)
	if strings.HasSuffix(filename, ".py") {
		return "python3 ${CLAUDE_PLUGIN_ROOT}/hooks/" + filename
	}
	return "bash ${CLAUDE_PLUGIN_ROOT}/hooks/" + filename
}

// nativeCheckAdvisorySuffix keeps `loaf check` hook commands aligned with the
// blocking flag in hooks.yaml: advisory hooks surface findings but exit 0.
func nativeCheckAdvisorySuffix(hook nativeBuildHook) string {
	if hook.section == "pre-tool" && !hook.blocking {
		return " --advisory"
	}
	return ""
}

var nativeClaudeBinaryPathHooks = map[string]bool{
	"artifact-body-write":     true,
	"check-" + "sec" + "rets": true,
	"ephemeral-provenance":    true,
	"github-account":          true,
	"render-drift":            true,
	"validate-push":           true,
	"validate-commit":         true,
	"workflow-pre-pr":         true,
	"security-audit":          true,
	"session-start-loaf":      true,
	"session-end-loaf":        true,
	"session-context-inject":  true,
	"pre-compact":             true,
	"post-compact":            true,
	"journal-post-commit":     true,
	"journal-post-pr":         true,
	"journal-post-merge":      true,
	"generate-task-board":     true,
	"journal-task-completed":  true,
	"detect-linear-magic":     true,
}

func nativeClaudeKnownCommands(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "dist", "skills"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var commands []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill := entry.Name()
		distSkill := filepath.Join(root, "dist", "skills", skill)
		if !pathExistsNative(filepath.Join(distSkill, "SKILL.md")) && !pathExistsNative(filepath.Join(distSkill, "references")) {
			continue
		}
		contentSkill := filepath.Join(root, "content", "skills", skill)
		if !pathExistsNative(contentSkill) {
			continue
		}
		extensions := parseNativeBuildSimpleYAMLScalars(readFileStringNative(filepath.Join(contentSkill, "SKILL.claude-code.yaml")))
		if extensions["user-invocable"] == "false" {
			continue
		}
		commands = append(commands, skill)
	}
	return commands, nil
}

func nativeClaudeScopeCommands(content string, commands []string) string {
	result := content
	for _, command := range commands {
		result = nativeClaudeScopeCommand(result, command)
	}
	return result
}

func nativeClaudeScopeCommand(content string, command string) string {
	needle := "/" + command
	var out strings.Builder
	offset := 0
	for {
		index := strings.Index(content[offset:], needle)
		if index < 0 {
			out.WriteString(content[offset:])
			break
		}
		index += offset
		out.WriteString(content[offset:index])
		after := index + len(needle)
		if nativeClaudeAlreadyScoped(content, index) || !nativeClaudeCommandBoundary(content, after) {
			out.WriteString(needle)
		} else {
			out.WriteString("/loaf:" + command)
		}
		offset = after
	}
	return out.String()
}

func nativeClaudeAlreadyScoped(content string, slashIndex int) bool {
	prefix := content[:slashIndex]
	lastSlash := strings.LastIndex(prefix, "/")
	if lastSlash < 0 {
		return false
	}
	scope := prefix[lastSlash+1:]
	if !strings.HasSuffix(scope, ":") || len(scope) < 2 {
		return false
	}
	scope = strings.TrimSuffix(scope, ":")
	for _, r := range scope {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return false
		}
	}
	return true
}

func nativeClaudeCommandBoundary(content string, index int) bool {
	if index >= len(content) {
		return true
	}
	switch content[index] {
	case ' ', '\n', '\t', '\r', ')', ']', ',', '`':
		return true
	default:
		return false
	}
}

func copyNativeClaudeHooks(hooks []nativeBuildHook, srcDir string, pluginDir string) error {
	hooksDir := filepath.Join(pluginDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := copyNativeBuildDir(filepath.Join(srcDir, "hooks", "lib"), filepath.Join(hooksDir, "lib"), nil, false); err != nil {
		return err
	}
	scriptIDs := map[string]nativeBuildHook{}
	for _, hook := range hooks {
		if hook.script == "" {
			continue
		}
		if hook.section != "session" && nativeClaudeBinaryPathHooks[hook.id] {
			continue
		}
		scriptIDs[hook.id] = hook
	}
	for _, id := range sortedNativeBuildMapKeys(scriptIDs) {
		hook := scriptIDs[id]
		src := filepath.Join(srcDir, hook.script)
		if pathExistsNative(src) {
			if err := copyNativeBuildFile(src, filepath.Join(hooksDir, filepath.Base(hook.script))); err != nil {
				return err
			}
		}
	}
	subagentSrc := filepath.Join(srcDir, "hooks", "subagent")
	entries, err := os.ReadDir(subagentSrc)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyNativeBuildFile(filepath.Join(subagentSrc, entry.Name()), filepath.Join(hooksDir, entry.Name())); err != nil {
			return err
		}
	}
	return copyNativeBuildDir(filepath.Join(srcDir, "hooks", "instructions"), filepath.Join(hooksDir, "instructions"), nil, false)
}

func copyNativeClaudeRuntimeFiles(root string, pluginDir string, version string) error {
	binDir := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if !pathExistsNative(filepath.Join(root, "bin", "loaf")) {
		return fmt.Errorf("Loaf launcher not found at bin/loaf. Run npm run build:go first.")
	}
	if err := copyNativeBuildFile(filepath.Join(root, "bin", "loaf"), filepath.Join(binDir, "loaf")); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(binDir, "loaf"), 0o755); err != nil {
		return err
	}
	if err := copyNativeBuildFile(filepath.Join(root, "bin", "package.json"), filepath.Join(binDir, "package.json")); err != nil {
		return err
	}
	if !pathExistsNative(filepath.Join(root, "bin", "native")) {
		return fmt.Errorf("Native Loaf runtime not found at bin/native/. Run npm run build:go first.")
	}
	if err := copyNativeBuildDir(filepath.Join(root, "bin", "native"), filepath.Join(binDir, "native"), nil, false); err != nil {
		return err
	}
	body, err := json.MarshalIndent(struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{
		Name:    nativeClaudePluginName,
		Version: version,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pluginDir, "package.json"), body, 0o644)
}

func nativeClaudeLSPJSON() string {
	return `{
  "go": {
    "command": "gopls",
    "args": [
      "serve"
    ],
    "extensionToLanguage": {
      ".go": "go"
    }
  },
  "python": {
    "command": "pyright-langserver",
    "args": [
      "--stdio"
    ],
    "extensionToLanguage": {
      ".py": "python",
      ".pyi": "python"
    }
  },
  "typescript": {
    "command": "typescript-language-server",
    "args": [
      "--stdio"
    ],
    "extensionToLanguage": {
      ".ts": "typescript",
      ".tsx": "typescriptreact",
      ".js": "javascript",
      ".jsx": "javascriptreact"
    }
  },
  "ruby": {
    "command": "solargraph",
    "args": [
      "stdio"
    ],
    "extensionToLanguage": {
      ".rb": "ruby",
      ".rake": "ruby",
      ".gemspec": "ruby"
    }
  }
}`
}

func readFileStringNative(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

func firstNonEmptyNativeClaude(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
