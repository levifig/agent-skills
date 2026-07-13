package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const loafConfigSchemaVersion = "1.0.0"

var configHookCommandRE = regexp.MustCompile(`loaf"?\s+check\s+--hook\s+([a-z0-9-]+)`)

type configCheckOptions struct {
	fix        bool
	jsonOutput bool
	help       bool
}

type configCheckResult struct {
	OK          bool                 `json:"ok"`
	Fixed       bool                 `json:"fixed"`
	ProjectRoot string               `json:"project_root"`
	Config      configFileStatus     `json:"config"`
	Targets     []configTargetStatus `json:"targets,omitempty"`
	Warnings    []string             `json:"warnings,omitempty"`
	Errors      []string             `json:"errors,omitempty"`
}

type configFileStatus struct {
	Path     string   `json:"path"`
	Status   string   `json:"status"`
	Missing  []string `json:"missing,omitempty"`
	Updated  []string `json:"updated,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

type configTargetStatus struct {
	Target       string   `json:"target"`
	ConfigDir    string   `json:"config_dir"`
	HookPath     string   `json:"hook_path,omitempty"`
	Status       string   `json:"status"`
	MissingHooks []string `json:"missing_hooks,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func (r Runner) runConfig(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 {
		writeConfigHelp(out)
		return nil
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		writeConfigHelp(out)
		return nil
	}
	switch args[0] {
	case "check":
		options, err := parseConfigCheckArgs(args[1:])
		if err != nil {
			return err
		}
		if options.help {
			writeConfigCheckHelp(out)
			return nil
		}
		projectRoot, err := project.ResolveRoot(runtimeRoot)
		if err != nil {
			return err
		}
		loafRoot, err := resolveLoafPackageRoot(r.WorkingDir, runtimeRoot)
		if err != nil {
			return err
		}
		result := runConfigCheck(projectRoot.Path(), loafRoot, options)
		if options.jsonOutput {
			if err := writeJSON(out, result); err != nil {
				return err
			}
		} else {
			writeConfigCheckText(out, result, options)
		}
		if !result.OK {
			return ExitError{Code: 2}
		}
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func parseConfigCheckArgs(args []string) (configCheckOptions, error) {
	var options configCheckOptions
	for _, arg := range args {
		switch arg {
		case "--fix":
			options.fix = true
		case "--json":
			options.jsonOutput = true
		case "--help", "-h":
			options.help = true
		default:
			return configCheckOptions{}, fmt.Errorf("unknown config check option %q", arg)
		}
	}
	return options, nil
}

func writeConfigHelp(out io.Writer) {
	writeUsageHelp(out, "loaf config <subcommand> [options]", "Validate and refresh project Loaf config.", "  check       Validate .agents/loaf.json and installed Loaf-managed hook config")
}

func writeConfigCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf config check [--fix] [--json]", "Validate .agents/loaf.json and installed Loaf-managed hook config.", "--fix       Create missing safe defaults and refresh stale installed target config", "--json      Output config status, target hook status, warnings, and errors as JSON")
}

func runConfigCheck(projectRoot string, loafRoot string, options configCheckOptions) configCheckResult {
	result := configCheckResult{
		OK:          true,
		ProjectRoot: projectRoot,
		Config:      checkProjectLoafConfig(projectRoot, options.fix, time.Now().UTC()),
	}
	result.Warnings = append(result.Warnings, result.Config.Warnings...)
	result.Errors = append(result.Errors, result.Config.Errors...)
	if len(result.Config.Errors) > 0 || result.Config.Status == "upgrade-needed" {
		result.OK = false
	}
	if result.Config.Status == "created" || result.Config.Status == "updated" {
		result.Fixed = true
	}

	targets := installedConfigTargets()
	for _, target := range targets {
		status := checkConfigTargetHooks(loafRoot, target)
		if len(status.MissingHooks) > 0 && options.fix {
			status = fixConfigTargetHooks(projectRoot, loafRoot, target, status)
		}
		if status.Status == "updated" {
			result.Fixed = true
		}
		if status.Error != "" || len(status.MissingHooks) > 0 {
			result.OK = false
			if status.Error != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", target.key, status.Error))
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: missing managed hook(s): %s", target.key, strings.Join(status.MissingHooks, ", ")))
			}
		}
		result.Targets = append(result.Targets, status)
	}
	return result
}

func checkProjectLoafConfig(projectRoot string, fix bool, now time.Time) configFileStatus {
	path := filepath.Join(projectRoot, ".agents", "loaf.json")
	status := configFileStatus{Path: ".agents/loaf.json", Status: "ok"}
	body, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			status.Status = "invalid"
			status.Errors = append(status.Errors, fmt.Sprintf("could not read .agents/loaf.json: %v", err))
			return status
		}
		status.Status = "missing"
		status.Errors = append(status.Errors, ".agents/loaf.json is missing")
		if !fix {
			return status
		}
		config := defaultLoafConfig(now)
		if err := writeLoafConfig(path, config); err != nil {
			status.Status = "invalid"
			status.Errors = []string{fmt.Sprintf("could not create .agents/loaf.json: %v", err)}
			return status
		}
		status.Status = "created"
		status.Errors = nil
		status.Updated = append(status.Updated, "version", "initialized", "knowledge", "integrations.linear.enabled", "integrations.serena.enabled")
		status.Warnings = append(status.Warnings, "integrations.github.account is not configured; github-account hook will pass through")
		return status
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		status.Status = "invalid"
		status.Errors = append(status.Errors, fmt.Sprintf("cannot parse .agents/loaf.json: %v", err))
		return status
	}
	config, ok := parsed.(map[string]any)
	if !ok || config == nil {
		status.Status = "invalid"
		status.Errors = append(status.Errors, ".agents/loaf.json must contain a JSON object")
		return status
	}

	updated, warnings, errors := ensureLoafConfigDefaults(config, now)
	status.Missing = append(status.Missing, updated...)
	status.Warnings = append(status.Warnings, warnings...)
	status.Errors = append(status.Errors, errors...)
	if len(status.Errors) > 0 {
		status.Status = "invalid"
		return status
	}
	if len(updated) > 0 {
		status.Status = "upgrade-needed"
		if fix {
			if err := writeLoafConfig(path, config); err != nil {
				status.Status = "invalid"
				status.Errors = append(status.Errors, fmt.Sprintf("could not update .agents/loaf.json: %v", err))
				return status
			}
			status.Status = "updated"
			status.Updated = append(status.Updated, updated...)
		}
	}
	return status
}

func defaultLoafConfig(now time.Time) map[string]any {
	return map[string]any{
		"version":     loafConfigSchemaVersion,
		"initialized": now.Format(time.RFC3339),
		"knowledge":   defaultNativeKBConfigJSON(),
		"integrations": map[string]any{
			"linear": map[string]any{"enabled": false},
			"serena": map[string]any{"enabled": false},
		},
	}
}

func ensureLoafConfigDefaults(config map[string]any, now time.Time) ([]string, []string, []string) {
	var updated []string
	var warnings []string
	var errors []string

	if value, ok := config["version"]; !ok {
		config["version"] = loafConfigSchemaVersion
		updated = append(updated, "version")
	} else if _, ok := value.(string); !ok {
		errors = append(errors, "version must be a string")
	}
	if value, ok := config["initialized"]; !ok {
		config["initialized"] = now.Format(time.RFC3339)
		updated = append(updated, "initialized")
	} else if _, ok := value.(string); !ok {
		errors = append(errors, "initialized must be a string")
	}

	knowledge, ok := config["knowledge"].(map[string]any)
	if !ok {
		if _, exists := config["knowledge"]; exists {
			errors = append(errors, "knowledge must be an object")
		} else {
			config["knowledge"] = defaultNativeKBConfigJSON()
			updated = append(updated, "knowledge")
		}
	} else {
		updated = append(updated, ensureKnowledgeConfigDefaults(knowledge, &errors)...)
	}

	integrations, ok := config["integrations"].(map[string]any)
	if !ok {
		if _, exists := config["integrations"]; exists {
			errors = append(errors, "integrations must be an object")
		} else {
			integrations = map[string]any{}
			config["integrations"] = integrations
			updated = append(updated, "integrations")
		}
	}
	if integrations != nil {
		updated = append(updated, ensureIntegrationEnabledDefault(integrations, "linear", false, &errors)...)
		updated = append(updated, ensureIntegrationEnabledDefault(integrations, "serena", false, &errors)...)
		warnings = append(warnings, validateGitHubIntegration(integrations, &errors)...)
	}

	return updated, warnings, errors
}

func ensureKnowledgeConfigDefaults(knowledge map[string]any, errors *[]string) []string {
	var updated []string
	if value, ok := knowledge["local"]; !ok {
		knowledge["local"] = []string{"docs/knowledge", "docs/decisions"}
		updated = append(updated, "knowledge.local")
	} else if !jsonArrayOfStrings(value) {
		*errors = append(*errors, "knowledge.local must be an array of strings")
	}
	if value, ok := knowledge["staleness_threshold_days"]; !ok {
		knowledge["staleness_threshold_days"] = float64(30)
		updated = append(updated, "knowledge.staleness_threshold_days")
	} else if !jsonNumber(value) {
		*errors = append(*errors, "knowledge.staleness_threshold_days must be a number")
	}
	if value, ok := knowledge["imports"]; !ok {
		knowledge["imports"] = []any{}
		updated = append(updated, "knowledge.imports")
	} else if _, ok := value.([]any); !ok {
		*errors = append(*errors, "knowledge.imports must be an array")
	}
	return updated
}

func ensureIntegrationEnabledDefault(integrations map[string]any, name string, enabled bool, errors *[]string) []string {
	section, ok := integrations[name].(map[string]any)
	if !ok {
		if _, exists := integrations[name]; exists {
			*errors = append(*errors, fmt.Sprintf("integrations.%s must be an object", name))
			return nil
		}
		integrations[name] = map[string]any{"enabled": enabled}
		return []string{"integrations." + name + ".enabled"}
	}
	if value, ok := section["enabled"]; !ok {
		section["enabled"] = enabled
		return []string{"integrations." + name + ".enabled"}
	} else if _, ok := value.(bool); !ok {
		*errors = append(*errors, fmt.Sprintf("integrations.%s.enabled must be a boolean", name))
	}
	return nil
}

func validateGitHubIntegration(integrations map[string]any, errors *[]string) []string {
	section, ok := integrations["github"].(map[string]any)
	if !ok {
		if _, exists := integrations["github"]; exists {
			*errors = append(*errors, "integrations.github must be an object")
			return nil
		}
		return []string{"integrations.github.account is not configured; github-account hook will pass through"}
	}
	account, ok := section["account"]
	if !ok || strings.TrimSpace(fmt.Sprint(account)) == "" {
		return []string{"integrations.github.account is not configured; github-account hook will pass through"}
	}
	if _, ok := account.(string); !ok {
		*errors = append(*errors, "integrations.github.account must be a string")
	}
	return nil
}

func writeLoafConfig(path string, config map[string]any) error {
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func jsonArrayOfStrings(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return false
		}
	}
	return true
}

func jsonNumber(value any) bool {
	switch value.(type) {
	case float64, int, int64, uint64:
		return true
	default:
		return false
	}
}

func installedConfigTargets() []detectedInstallTool {
	tools := detectInstallTools()
	for i := range tools {
		if record, ok := readConfigInstallRecord(tools[i].key); ok {
			if record.ConfigDir != "" {
				tools[i].configDir = record.ConfigDir
			}
			tools[i].installed = true
		}
	}
	var installed []detectedInstallTool
	for _, tool := range tools {
		if tool.installed {
			installed = append(installed, tool)
		}
	}
	sort.Slice(installed, func(i, j int) bool { return installed[i].key < installed[j].key })
	return installed
}

func readConfigInstallRecord(target string) (installTargetRecord, bool) {
	body, err := os.ReadFile(installRecordPath(installHome(), target))
	if err != nil {
		return installTargetRecord{}, false
	}
	var record installTargetRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return installTargetRecord{}, false
	}
	return record, true
}

func checkConfigTargetHooks(loafRoot string, target detectedInstallTool) configTargetStatus {
	status := configTargetStatus{
		Target:    target.key,
		ConfigDir: target.configDir,
		HookPath:  configTargetHookPath(target.key, target.configDir),
		Status:    "ok",
	}
	expectedPath := configDistributionHookPath(loafRoot, target.key)
	expected, err := configHookIDsFromFile(expectedPath)
	if err != nil {
		status.Status = "error"
		status.Error = fmt.Sprintf("cannot read current hook distribution %s: %v", displayConfigPath(expectedPath), err)
		return status
	}
	actual, err := configHookIDsFromFile(status.HookPath)
	if err != nil {
		status.Status = "stale"
		status.MissingHooks = expected
		return status
	}
	status.MissingHooks = missingConfigHookIDs(expected, actual)
	if len(status.MissingHooks) > 0 {
		status.Status = "stale"
	}
	return status
}

func fixConfigTargetHooks(projectRoot string, loafRoot string, target detectedInstallTool, previous configTargetStatus) configTargetStatus {
	return fixConfigTargetHooksWithInstaller(projectRoot, loafRoot, target, previous, installTargetDistribution)
}

func fixConfigTargetHooksWithInstaller(projectRoot string, loafRoot string, target detectedInstallTool, previous configTargetStatus, installer func(targetInstallOptions) error) configTargetStatus {
	distDir := filepath.Join(loafRoot, "dist", target.key)
	if !dirExistsForInstall(distDir) {
		previous.Status = "error"
		previous.Error = fmt.Sprintf("no build output found for %s; run `loaf build` or reinstall Loaf", target.key)
		return previous
	}
	if err := installer(targetInstallOptions{
		Target:      target.key,
		DistDir:     distDir,
		ConfigDir:   target.configDir,
		Upgrade:     true,
		Version:     packageVersion(loafRoot),
		HomeDir:     installHome(),
		CodexHome:   os.Getenv("CODEX_HOME"),
		ProjectRoot: projectRoot,
	}); err != nil {
		previous.Status = "error"
		previous.Error = err.Error()
		return previous
	}
	updated := checkConfigTargetHooks(loafRoot, target)
	if len(updated.MissingHooks) == 0 && updated.Error == "" {
		updated.Status = "updated"
	}
	return updated
}

func configDistributionHookPath(loafRoot string, target string) string {
	switch target {
	case "cursor":
		return filepath.Join(loafRoot, "dist", "cursor", "hooks.json")
	case "codex":
		return filepath.Join(loafRoot, "dist", "codex", ".codex", "hooks.json")
	case "opencode":
		return filepath.Join(loafRoot, "dist", "opencode", "plugins", "hooks.ts")
	case "amp":
		return filepath.Join(loafRoot, "dist", "amp", ".amp", "plugins", "loaf.ts")
	default:
		return ""
	}
}

func configTargetHookPath(target string, configDir string) string {
	switch target {
	case "cursor", "codex":
		return filepath.Join(configDir, "hooks.json")
	case "opencode":
		return filepath.Join(configDir, "plugins", "hooks.ts")
	case "amp":
		return filepath.Join(configDir, "plugins", "loaf.ts")
	default:
		return ""
	}
}

func configHookIDsFromFile(path string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("unknown hook path")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	matches := configHookCommandRE.FindAllStringSubmatch(string(body), -1)
	seen := map[string]bool{}
	for _, match := range matches {
		if len(match) == 2 {
			seen[match[1]] = true
		}
	}
	if strings.Contains(string(body), codexJournalHookCommandSuffix) {
		seen["codex-session-start"] = true
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func missingConfigHookIDs(expected []string, actual []string) []string {
	actualSet := map[string]bool{}
	for _, id := range actual {
		actualSet[id] = true
	}
	var missing []string
	for _, id := range expected {
		if !actualSet[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func displayConfigPath(path string) string {
	if rel, err := filepath.Rel(".", path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func writeConfigCheckText(out io.Writer, result configCheckResult, options configCheckOptions) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf config check"))
	fmt.Fprintln(out)
	writeConfigFileStatusText(out, result.Config)
	for _, target := range result.Targets {
		writeConfigTargetStatusText(out, target)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "  %s %s\n", ansiYellow("!"), warning)
	}
	if !result.OK {
		for _, configErr := range result.Errors {
			fmt.Fprintf(out, "  %s %s\n", ansiRed("x"), configErr)
		}
		if !options.fix {
			fmt.Fprintf(out, "  %s Run %s to apply safe updates.\n", ansiGray("next:"), ansiBold("loaf config check --fix"))
		}
		fmt.Fprintln(out)
		return
	}
	if result.Fixed {
		fmt.Fprintf(out, "  %s Config is valid and current after fixes.\n\n", ansiGreen("✓"))
	} else {
		fmt.Fprintf(out, "  %s Config is valid and current.\n\n", ansiGreen("✓"))
	}
}

func writeConfigFileStatusText(out io.Writer, status configFileStatus) {
	switch status.Status {
	case "ok":
		fmt.Fprintf(out, "  %s %s valid\n", ansiGreen("✓"), status.Path)
	case "created":
		fmt.Fprintf(out, "  %s %s created\n", ansiGreen("+"), status.Path)
	case "updated":
		fmt.Fprintf(out, "  %s %s updated (%s)\n", ansiGreen("✓"), status.Path, strings.Join(status.Updated, ", "))
	case "upgrade-needed":
		fmt.Fprintf(out, "  %s %s needs defaults: %s\n", ansiYellow("!"), status.Path, strings.Join(status.Missing, ", "))
	case "missing", "invalid":
		fmt.Fprintf(out, "  %s %s %s\n", ansiRed("x"), status.Path, status.Status)
	}
}

func writeConfigTargetStatusText(out io.Writer, status configTargetStatus) {
	switch status.Status {
	case "ok":
		fmt.Fprintf(out, "  %s %s hooks current\n", ansiGreen("✓"), installDisplayName(status.Target))
	case "updated":
		fmt.Fprintf(out, "  %s %s hooks refreshed\n", ansiGreen("✓"), installDisplayName(status.Target))
	case "stale":
		fmt.Fprintf(out, "  %s %s hooks missing: %s\n", ansiYellow("!"), installDisplayName(status.Target), strings.Join(status.MissingHooks, ", "))
	case "error":
		fmt.Fprintf(out, "  %s %s hooks: %s\n", ansiRed("x"), installDisplayName(status.Target), status.Error)
	}
}
