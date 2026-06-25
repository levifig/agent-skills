package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	loafInstallMarkerFile = ".loaf-version"
	loafHookMarker        = "loaf-managed"
)

var legacyLoafHookSignatures = map[string]bool{
	"command:loaf check --hook check-" + "se" + "crets|matcher:Edit|Write|Bash|if:":           true,
	"command:loaf check --hook security-audit|matcher:Bash|if:":                               true,
	"command:loaf check --hook validate-push|matcher:Bash|if:":                                true,
	"command:loaf check --hook workflow-pre-pr|matcher:Bash|if:":                              true,
	"command:loaf check --hook validate-commit|matcher:Bash|if:":                              true,
	"command:loaf session log --detect-linear|matcher:Bash|if:":                               true,
	"command:loaf task refresh|matcher:Edit|Write|if:":                                        true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(git commit:*)":                 true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr create:*)":               true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)":                true,
	"command:loaf session start|matcher:|if:":                                                 true,
	"command:loaf session end|matcher:|if:":                                                   true,
	"command:bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh|matcher:Edit|Write|if:": true,
	"command:bash $HOME/.cursor/hooks/session/compact.sh|matcher:|if:":                        true,
}

var legacyLoafCommands = map[string]bool{
	"bash $HOME/.cursor/hooks/session/session-start-soul.sh":  true,
	"bash $HOME/.cursor/hooks/session/session-start.sh":       true,
	"bash $HOME/.cursor/hooks/session/kb-session-start.sh":    true,
	"bash $HOME/.cursor/hooks/session/session-end.sh":         true,
	"bash $HOME/.cursor/hooks/session/kb-session-end.sh":      true,
	"bash $HOME/.cursor/hooks/session/pre-compact-archive.sh": true,
}

var legacyLoafPromptPrefixes = []string{
	"STOP. Before running gh pr merge",
	"ADVISORY: You are about to run `git push`",
	"KNOWLEDGE BASE:",
	"POST-MERGE HOUSEKEEPING:",
	"CONTEXT COMPACTION IMMINENT:",
	"SESSION JOURNAL NUDGE:",
}

var obsoleteCursorHookFiles = []string{
	"session/check-sessions.sh",
	"session/kb-session-end.sh",
	"session/kb-session-start.sh",
	"session/pre-compact-archive.sh",
	"session/session-end-simple.sh",
	"session/session-end.sh",
	"session/session-start-soul.sh",
	"session/session-start.sh",
}

type targetInstallOptions struct {
	Target        string
	DistDir       string
	ConfigDir     string
	Upgrade       bool
	Version       string
	HomeDir       string
	CodexHome     string
	AmpSkillsDir  string
	AmpPluginsDir string
}

type codexHooksFile struct {
	Version int                         `json:"version,omitempty"`
	Hooks   map[string][]map[string]any `json:"hooks,omitempty"`
}

func installTargetDistribution(options targetInstallOptions) error {
	if options.Target == "" {
		return fmt.Errorf("install target is required")
	}
	if options.DistDir == "" {
		return fmt.Errorf("install dist dir is required")
	}
	if options.ConfigDir == "" {
		return fmt.Errorf("install config dir is required")
	}
	if options.Version == "" {
		options.Version = "0.0.0"
	}

	switch options.Target {
	case "opencode":
		return installOpencodeTarget(options)
	case "cursor":
		return installCursorTarget(options)
	case "codex":
		return installCodexTarget(options)
	case "amp":
		return installAmpTarget(options)
	default:
		return fmt.Errorf("no installer available for target %q", options.Target)
	}
}

func installOpencodeTarget(options targetInstallOptions) error {
	for _, dir := range []string{"skills", "agents", "commands", "plugins", "templates"} {
		if err := syncTargetSubdir(options.DistDir, options.ConfigDir, dir); err != nil {
			return err
		}
	}
	return writeInstallMarker(options.ConfigDir, options.Version)
}

func installCursorTarget(options targetInstallOptions) error {
	homeDir := installHomeDir(options)
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "skills"), filepath.Join(homeDir, ".agents", "skills")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(options.ConfigDir, "commands")); err != nil {
		return err
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "agents"), filepath.Join(options.ConfigDir, "agents")); err != nil {
		return err
	}
	if err := mergeHookFiles(filepath.Join(options.ConfigDir, "hooks.json"), filepath.Join(options.DistDir, "hooks.json")); err != nil {
		return err
	}
	if options.Upgrade {
		for _, file := range obsoleteCursorHookFiles {
			if err := os.Remove(filepath.Join(options.ConfigDir, "hooks", filepath.FromSlash(file))); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := mergeTargetDirIfExists(filepath.Join(options.DistDir, "hooks"), filepath.Join(options.ConfigDir, "hooks")); err != nil {
		return err
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "templates"), filepath.Join(options.ConfigDir, "templates")); err != nil {
		return err
	}
	return writeInstallMarker(options.ConfigDir, options.Version)
}

func installCodexTarget(options targetInstallOptions) error {
	homeDir := installHomeDir(options)
	codexHome := options.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "skills"), filepath.Join(homeDir, ".agents", "skills")); err != nil {
		return err
	}
	if err := mergeHookFiles(filepath.Join(codexHome, "hooks.json"), filepath.Join(options.DistDir, ".codex", "hooks.json")); err != nil {
		return err
	}
	return writeInstallMarker(options.ConfigDir, options.Version)
}

func installAmpTarget(options targetInstallOptions) error {
	homeDir := installHomeDir(options)
	skillsDest := options.AmpSkillsDir
	if skillsDest == "" {
		skillsDest = filepath.Join(homeDir, ".config", "agents", "skills")
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
		return err
	}
	pluginSrc := filepath.Join(options.DistDir, ".amp", "plugins", "loaf.ts")
	if fileExistsForInstall(pluginSrc) {
		pluginsDest := options.AmpPluginsDir
		if pluginsDest == "" {
			pluginsDest = filepath.Join(homeDir, ".amp", "plugins")
		}
		if err := os.MkdirAll(pluginsDest, 0o755); err != nil {
			return err
		}
		if err := copyFileForInstall(pluginSrc, filepath.Join(pluginsDest, "loaf.ts")); err != nil {
			return err
		}
	}
	return writeInstallMarker(options.ConfigDir, options.Version)
}

func installHomeDir(options targetInstallOptions) string {
	if options.HomeDir != "" {
		return options.HomeDir
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	return os.Getenv("USERPROFILE")
}

func syncTargetSubdir(distDir string, configDir string, name string) error {
	return syncTargetDirIfExists(filepath.Join(distDir, name), filepath.Join(configDir, name))
}

func syncTargetDirIfExists(src string, dest string) error {
	if !dirExistsForInstall(src) {
		return nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	return copyDirContentsForInstall(src, dest)
}

func mergeTargetDirIfExists(src string, dest string) error {
	if !dirExistsForInstall(src) {
		return nil
	}
	return copyDirContentsForInstall(src, dest)
}

func copyDirContentsForInstall(src string, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFileWithModeForInstall(path, target, info.Mode().Perm())
	})
}

func copyFileForInstall(src string, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileWithModeForInstall(src, dest, info.Mode().Perm())
}

func copyFileWithModeForInstall(src string, dest string, mode fs.FileMode) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, mode)
}

func writeInstallMarker(configDir string, version string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, loafInstallMarkerFile), []byte(version+"\n"), 0o644)
}

func mergeHookFiles(destPath string, loafPath string) error {
	if !fileExistsForInstall(loafPath) {
		return nil
	}
	existing, err := loadCodexHooksFile(destPath)
	if err != nil {
		return err
	}
	loafHooks, err := loadCodexHooksFile(loafPath)
	if err != nil {
		return err
	}
	merged := codexHooksFile{Version: 1, Hooks: map[string][]map[string]any{}}
	seen := map[string]bool{}
	for hookType := range existing.Hooks {
		seen[hookType] = true
	}
	for hookType := range loafHooks.Hooks {
		seen[hookType] = true
	}
	for hookType := range seen {
		var hooks []map[string]any
		for _, hook := range existing.Hooks[hookType] {
			if !isLoafInstallHook(hook) {
				hooks = append(hooks, hook)
			}
		}
		hooks = append(hooks, loafHooks.Hooks[hookType]...)
		if len(hooks) > 0 {
			merged.Hooks[hookType] = hooks
		}
	}
	if len(merged.Hooks) == 0 {
		merged.Hooks = nil
	}
	body, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, body, 0o644)
}

func loadCodexHooksFile(path string) (codexHooksFile, error) {
	if !fileExistsForInstall(path) {
		return codexHooksFile{Version: 1, Hooks: map[string][]map[string]any{}}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return codexHooksFile{}, err
	}
	var hooks codexHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return codexHooksFile{Version: 1, Hooks: map[string][]map[string]any{}}, nil
	}
	if hooks.Version == 0 {
		hooks.Version = 1
	}
	if hooks.Hooks == nil {
		hooks.Hooks = map[string][]map[string]any{}
	}
	return hooks, nil
}

func isLoafInstallHook(hook map[string]any) bool {
	if marker, ok := hook[loafHookMarker].(bool); ok && marker {
		return true
	}
	if signature := installHookSignature(hook); signature != "" && legacyLoafHookSignatures[signature] {
		return true
	}
	if command, ok := hook["command"].(string); ok && legacyLoafCommands[command] {
		return true
	}
	if prompt, ok := hook["prompt"].(string); ok {
		for _, prefix := range legacyLoafPromptPrefixes {
			if strings.HasPrefix(prompt, prefix) {
				return true
			}
		}
	}
	return false
}

func installHookSignature(hook map[string]any) string {
	command, hasCommand := hook["command"].(string)
	prompt, hasPrompt := hook["prompt"].(string)
	matcher, _ := hook["matcher"].(string)
	condition, _ := hook["if"].(string)
	switch {
	case hasCommand:
		return fmt.Sprintf("command:%s|matcher:%s|if:%s", command, matcher, condition)
	case hasPrompt:
		return fmt.Sprintf("prompt:%s|matcher:%s|if:%s", prompt, matcher, condition)
	default:
		return ""
	}
}

func dirExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
