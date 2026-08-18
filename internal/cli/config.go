package cli

import (
	"context"
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
	"github.com/levifig/loaf/internal/state"
)

const loafConfigSchemaVersion = "1.0.0"

var configHookCommandRE = regexp.MustCompile(`loaf"?\s+check\s+--hook\s+([a-z0-9-]+)`)

type configCheckOptions struct {
	fix        bool
	jsonOutput bool
	help       bool
	// hookState reaches the enablement records for targets whose hook entries
	// are reconciled. Diagnosis crosses those records with the live file, and
	// `--fix` drives the shared installer, which converges the entries against
	// them rather than overwriting the file — so the resolver has to travel with
	// the request. Which resolver it is matters: a plain check gets the
	// read-only one, because a diagnosis that created a database would be a
	// write, and this command promises none without `--fix`.
	hookState hookStateResolver
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

// configTargetStatus is one installed harness's hook health. Targets whose
// entries are reconciled report Hooks — one state per catalog identity — and
// the rest report MissingHooks, the whole-file question their plugin surfaces
// still ask. Neither list ever mentions an entry Loaf does not own.
type configTargetStatus struct {
	Target       string          `json:"target"`
	ConfigDir    string          `json:"config_dir"`
	HookPath     string          `json:"hook_path,omitempty"`
	Status       string          `json:"status"`
	MissingHooks []string        `json:"missing_hooks,omitempty"`
	Hooks        []hookDiagnosis `json:"hooks,omitempty"`
	Error        string          `json:"error,omitempty"`
}

func (s configTargetStatus) healthy() bool {
	if s.Error != "" || len(s.MissingHooks) > 0 {
		return false
	}
	for _, hook := range s.Hooks {
		if !hook.healthy() {
			return false
		}
	}
	return true
}

// remedyPhrases names what this target needs, one phrase per distinct remedy so
// a reconcile and a reprojection are not blurred into a single "stale". A
// healthy target has none.
func (s configTargetStatus) remedyPhrases() []string {
	if len(s.MissingHooks) > 0 {
		return []string{"hooks missing: " + strings.Join(s.MissingHooks, ", ")}
	}
	var phrases []string
	for _, remedy := range []string{"reconcile", "reprojection"} {
		var named []string
		for _, hook := range s.Hooks {
			if hook.remedy() == remedy {
				named = append(named, fmt.Sprintf("%s (%s)", hook.HookID, strings.ReplaceAll(hook.State, "-", " ")))
			}
		}
		if len(named) > 0 {
			phrases = append(phrases, "hooks need "+remedy+": "+strings.Join(named, ", "))
		}
	}
	return phrases
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
		loafRoot, err := r.resolveInstalledDistributionRoot()
		if err != nil {
			return err
		}
		resolveHookState := r.hookStateForPlan
		if options.fix {
			resolveHookState = r.hookStateForApply
		}
		hookState, releaseHookState := resolveHookState(projectRoot.Path())
		defer releaseHookState()
		options.hookState = hookState
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

	for _, target := range installedConfigTargets() {
		status := checkConfigTargetHooks(projectRoot, loafRoot, target, options.hookState)
		if !status.healthy() && options.fix {
			status = fixConfigTargetHooks(projectRoot, loafRoot, target, status, options.hookState)
		}
		if status.Status == "updated" {
			result.Fixed = true
		}
		if !status.healthy() {
			result.OK = false
			detail := status.Error
			if detail == "" {
				detail = strings.Join(status.remedyPhrases(), "; ")
			}
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", target.key, detail))
		}
		result.Targets = append(result.Targets, status)
	}
	return result
}

func checkProjectLoafConfig(projectRoot string, fix bool, now time.Time) configFileStatus {
	path := filepath.Join(projectRoot, ".agents", "loaf.json")
	status := configFileStatus{Path: ".agents/loaf.json", Status: "ok"}
	body, err := readRegularFile(path, projectFileReadLimit)
	if err != nil {
		if !os.IsNotExist(err) {
			status.Status = "invalid"
			status.Errors = append(status.Errors, fmt.Sprintf("could not read .agents/loaf.json: %v", refuseProjectFileRead(err)))
			return status
		}
		status.Status = "missing"
		status.Errors = append(status.Errors, ".agents/loaf.json is missing")
		if !fix {
			return status
		}
		config := defaultLoafConfig(now, projectRoot)
		if err := writeLoafConfig(path, config); err != nil {
			status.Status = "invalid"
			status.Errors = []string{fmt.Sprintf("could not create .agents/loaf.json: %v", err)}
			return status
		}
		status.Status = "created"
		status.Errors = nil
		status.Updated = append(status.Updated, "version", "initialized", "knowledge", "integrations.linear.enabled", "integrations.serena.enabled", "issue")
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

func defaultIssueConfig(projectRoot string) map[string]any {
	prefix := state.DeriveIssuePrefix("", projectRoot)
	if prefix == "" {
		prefix = state.DefaultIssuePrefix
	}
	return map[string]any{
		"authority": state.IssueAuthorityLocal,
		"prefix":    prefix,
	}
}

func defaultLoafConfig(now time.Time, projectRoot string) map[string]any {
	return map[string]any{
		"version":     loafConfigSchemaVersion,
		"initialized": now.Format(time.RFC3339),
		"knowledge":   defaultNativeKBConfigJSON(),
		"integrations": map[string]any{
			"linear": map[string]any{"enabled": false},
			"serena": map[string]any{"enabled": false},
		},
		"issue": defaultIssueConfig(projectRoot),
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
		validateIntegrationMCPServerName(integrations, "linear", &errors)
		warnings = append(warnings, validateGitHubIntegration(integrations, &errors)...)
	}

	issueWarnings, issueErrors := state.IssueProjectConfigFindings(config)
	warnings = append(warnings, issueWarnings...)
	errors = append(errors, issueErrors...)

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

func validateIntegrationMCPServerName(integrations map[string]any, name string, errors *[]string) {
	section, ok := integrations[name].(map[string]any)
	if !ok {
		return
	}
	value, exists := section["mcp_server_name"]
	if !exists {
		return
	}
	serverName, ok := value.(string)
	if !ok || strings.TrimSpace(serverName) == "" {
		*errors = append(*errors, fmt.Sprintf("integrations.%s.mcp_server_name must be a nonempty string", name))
	}
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
	body, err := readRegularFile(installRecordPath(installHome(), target), projectFileReadLimit)
	if err != nil {
		return installTargetRecord{}, false
	}
	var record installTargetRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return installTargetRecord{}, false
	}
	return record, true
}

// checkConfigTargetHooks reports one installed harness's hook health. A target
// whose entries are reconciled is diagnosed per identity against the records and
// the live file; the plugin targets keep the whole-file question, which is the
// right one for a file Loaf writes outright.
func checkConfigTargetHooks(projectRoot string, loafRoot string, target detectedInstallTool, hookState hookStateResolver) configTargetStatus {
	if targetKeepsReconciledHookFile(target.key) {
		return diagnoseConfigTargetHooks(projectRoot, loafRoot, target, hookState)
	}
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

// diagnoseConfigTargetHooks is the enablement-aware half. It runs the
// reconciler's own read — same catalog, same recognition, same pairing — so the
// verdict here and the actions an upgrade would take cannot disagree, and a
// hook the operator disabled reads healthy rather than missing.
func diagnoseConfigTargetHooks(projectRoot string, loafRoot string, target detectedInstallTool, hookState hookStateResolver) configTargetStatus {
	options := configTargetInstallOptions(projectRoot, loafRoot, target, hookState, false)
	status := configTargetStatus{
		Target:    target.key,
		ConfigDir: target.configDir,
		HookPath:  targetHookFilePath(options),
		Status:    "ok",
	}
	reconciler, err := hooksReconcilerFor(options)
	if err == nil {
		status.Hooks, err = reconciler.diagnose(context.Background())
	}
	if err != nil {
		status.Status = "error"
		status.Error = err.Error()
		return status
	}
	if !status.healthy() {
		status.Status = "stale"
	}
	return status
}

func fixConfigTargetHooks(projectRoot string, loafRoot string, target detectedInstallTool, previous configTargetStatus, hookState hookStateResolver) configTargetStatus {
	return fixConfigTargetHooksWithInstaller(projectRoot, loafRoot, target, previous, hookState, installTargetDistribution)
}

// fixConfigTargetHooksWithInstaller converges through the shared installer and
// nothing else. For a reconciled target that installer is the reconciler, so
// `--fix` adds a hook the operator never disabled, removes one they did, and
// converges a drifted entry — by the same path an upgrade takes, with no
// private refresh of its own.
func fixConfigTargetHooksWithInstaller(projectRoot string, loafRoot string, target detectedInstallTool, previous configTargetStatus, hookState hookStateResolver, installer func(targetInstallOptions) error) configTargetStatus {
	options := configTargetInstallOptions(projectRoot, loafRoot, target, hookState, true)
	if !dirExistsForInstall(options.DistDir) {
		previous.Status = "error"
		previous.Error = fmt.Sprintf("no build output found for %s; run `loaf build` or reinstall Loaf", target.key)
		return previous
	}
	if err := installer(options); err != nil {
		previous.Status = "error"
		previous.Error = err.Error()
		return previous
	}
	updated := checkConfigTargetHooks(projectRoot, loafRoot, target, hookState)
	if updated.healthy() {
		updated.Status = "updated"
	}
	return updated
}

// configTargetInstallOptions is how this command addresses one installed
// harness. Diagnosis and `--fix` build it the same way so the target read is
// the target written.
func configTargetInstallOptions(projectRoot string, loafRoot string, target detectedInstallTool, hookState hookStateResolver, upgrade bool) targetInstallOptions {
	return targetInstallOptions{
		Target:      target.key,
		DistDir:     filepath.Join(loafRoot, "dist", target.key),
		ConfigDir:   target.configDir,
		Upgrade:     upgrade,
		Version:     packageVersion(loafRoot),
		HomeDir:     installHome(),
		CodexHome:   os.Getenv("CODEX_HOME"),
		ProjectRoot: projectRoot,
		HookState:   hookState,
	}
}

func configDistributionHookPath(loafRoot string, target string) string {
	switch target {
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
	body, err := readRegularFile(path, projectFileReadLimit)
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
		for _, phrase := range status.remedyPhrases() {
			fmt.Fprintf(out, "  %s %s %s\n", ansiYellow("!"), installDisplayName(status.Target), phrase)
		}
	case "error":
		fmt.Fprintf(out, "  %s %s hooks: %s\n", ansiRed("x"), installDisplayName(status.Target), status.Error)
	}
}
