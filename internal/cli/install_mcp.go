package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type installMcpDefinition struct {
	id          string
	displayName string
	tier        string
	defaultArgs []string
	targetArgs  map[string][]string
}

type installMcpTargetConfig struct {
	globalPath  string
	projectPath string
	mcpKey      string
	format      string
}

type installMcpTargetStatus struct {
	configured bool
	scope      string
}

var installMcpDefinitions = []installMcpDefinition{
	{
		id:          "linear",
		displayName: "Linear",
		tier:        "recommended",
		defaultArgs: []string{"npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"},
	},
	{
		id:          "serena",
		displayName: "Serena",
		tier:        "optional",
		defaultArgs: []string{"serena", "start-mcp-server", "--project-from-cwd"},
		targetArgs: map[string][]string{
			"claude-code": {"serena", "start-mcp-server", "--context", "claude-code", "--project-from-cwd"},
			"cursor":      {"serena", "start-mcp-server", "--context", "ide", "--project-from-cwd"},
			"codex":       {"serena", "start-mcp-server", "--context", "codex", "--project-from-cwd"},
		},
	},
}

var installMcpTargetConfigs = map[string]installMcpTargetConfig{
	"claude-code": {globalPath: "~/.claude/settings.json", projectPath: ".claude/settings.json", mcpKey: "mcpServers", format: "json"},
	"cursor":      {globalPath: "~/.cursor/mcp.json", projectPath: ".cursor/mcp.json", mcpKey: "mcpServers", format: "json"},
	"opencode":    {globalPath: "~/.config/opencode/opencode.json", projectPath: "opencode.json", mcpKey: "mcp", format: "json"},
	"codex":       {globalPath: "~/.codex/config.toml", projectPath: ".codex/config.toml", mcpKey: "mcp_servers", format: "toml"},
	"gemini":      {globalPath: "~/.gemini/settings.json", projectPath: ".gemini/settings.json", mcpKey: "mcpServers", format: "json"},
	"amp":         {globalPath: "~/.config/amp/settings.json", projectPath: ".amp/settings.json", mcpKey: "amp.mcpServers", format: "json"},
}

var serenaInstallCommand = []string{"tool", "install", "-p", "3.13", "serena-agent@latest", "--prerelease=allow"}

func (r Runner) runInstallMcpRecommendations(out io.Writer, projectRoot string, upgrade bool, availableTargets []string) error {
	if upgrade {
		return nil
	}
	availableTargets = uniqueInstallTargets(availableTargets)
	if !r.installMcpInteractive() {
		return recordDefaultInstallMcpChoices(projectRoot)
	}
	if len(availableTargets) == 0 {
		return nil
	}

	input := r.Stdin
	if input == nil {
		input = os.Stdin
	}
	reader := bufio.NewReader(input)
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("Recommended MCP servers"))
	for _, def := range installMcpDefinitions {
		tierLabel := ""
		if def.tier == "optional" {
			tierLabel = ansiGray("Optional - ")
		}
		fmt.Fprintf(out, "  %s%s\n", tierLabel, ansiBold(def.displayName))
		statuses := buildInstallMcpStatuses(projectRoot, availableTargets, def)
		for _, target := range availableTargets {
			fmt.Fprintln(out, formatInstallMcpTargetLine(target, statuses[target]))
		}
		if installMcpDoneForTargets(statuses, availableTargets) {
			if err := mergeInstallLoafConfigIntegrations(projectRoot, map[string]bool{def.id: true}, false); err != nil {
				return err
			}
			fmt.Fprintln(out)
			continue
		}

		scopeAnswer, err := askInstallLine(reader, out, "    Install? [n]o / [g]lobal / [p]roject (default: no): ")
		if err != nil {
			return err
		}
		scope := parseInstallMcpScope(scopeAnswer)
		if scope == "no" {
			if err := mergeInstallLoafConfigIntegrations(projectRoot, map[string]bool{def.id: false}, false); err != nil {
				return err
			}
			fmt.Fprintf(out, "    %s Skipped - recorded in .agents/loaf.json\n\n", ansiGray("○"))
			continue
		}
		if def.id == "serena" {
			result := ensureSerenaNativeInstall(reader, out)
			if !result.ok {
				if err := mergeInstallLoafConfigIntegrations(projectRoot, map[string]bool{def.id: false}, false); err != nil {
					return err
				}
				fmt.Fprintf(out, "    %s %s\n\n", ansiYellow("⚠"), result.message)
				continue
			}
			fmt.Fprintf(out, "    %s %s\n", ansiGreen("✓"), result.message)
		}

		unconfigured := unconfiguredInstallMcpTargets(availableTargets, statuses)
		selectedTargets := unconfigured
		if len(unconfigured) > 1 {
			fmt.Fprintln(out, "\n    Unconfigured targets:")
			for i, target := range unconfigured {
				fmt.Fprintf(out, "      %d. %s\n", i+1, target)
			}
			targetAnswer, err := askInstallLine(reader, out, "    Which targets? [a]ll / comma-separated numbers: ")
			if err != nil {
				return err
			}
			selectedTargets = parseInstallMcpTargetSelection(targetAnswer, unconfigured)
		}

		allOK := true
		for _, target := range selectedTargets {
			result := installMcpForTarget(target, def, scope, projectRoot)
			if result.ok {
				fmt.Fprintf(out, "    %s %s: %s\n", ansiGreen("✓"), target, result.message)
			} else {
				allOK = false
				fmt.Fprintf(out, "    %s %s: %s\n", ansiYellow("⚠"), target, result.message)
			}
		}
		alreadyConfigured := 0
		for _, target := range availableTargets {
			if statuses[target].configured {
				alreadyConfigured++
			}
		}
		enabled := allOK && alreadyConfigured+len(selectedTargets) == len(availableTargets)
		if err := mergeInstallLoafConfigIntegrations(projectRoot, map[string]bool{def.id: enabled}, false); err != nil {
			return err
		}
		if !enabled {
			fmt.Fprintf(out, "    %s %s\n", ansiYellow("⚠"), ansiGray("integration remains disabled until all selected targets succeed."))
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "%s Integration choices recorded under %s in %s\n\n", ansiGreen("✓"), ansiGray("integrations"), ansiGray(".agents/loaf.json"))
	return nil
}

func (r Runner) installMcpInteractive() bool {
	if r.Stdin != nil {
		return true
	}
	stat, err := os.Stdin.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

func recordDefaultInstallMcpChoices(projectRoot string) error {
	updates := map[string]bool{}
	existing := readInstallLoafConfig(projectRoot)
	integrations, _ := existing["integrations"].(map[string]any)
	for _, def := range installMcpDefinitions {
		if integrations == nil || integrations[def.id] == nil {
			updates[def.id] = false
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return mergeInstallLoafConfigIntegrations(projectRoot, updates, true)
}

func askInstallLine(reader *bufio.Reader, out io.Writer, question string) (string, error) {
	fmt.Fprint(out, question)
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(strings.ToLower(answer)), nil
}

func ensureSerenaNativeInstall(reader *bufio.Reader, out io.Writer) installMcpResult {
	if installCommandExists("serena") {
		return installMcpResult{ok: true, message: "Serena native CLI found"}
	}

	printSerenaNativeInstructions(out)
	if !installCommandExists("uv") {
		return installMcpResult{message: "uv not found; install uv, run the Serena setup commands above, then rerun loaf install."}
	}

	yes, err := askInstallYesNo(reader, out, "    Run Serena native install now? [y/N] ", false)
	if err != nil {
		return installMcpResult{message: "Could not read Serena install prompt: " + err.Error()}
	}
	if !yes {
		return installMcpResult{message: "Skipped native Serena setup; MCP config not written."}
	}

	if result := runInstallCommand("uv", serenaInstallCommand, out); !result.ok {
		return installMcpResult{message: "Serena install failed: " + result.message}
	}
	if !installCommandExists("serena") {
		return installMcpResult{message: "Serena installed, but `serena` is not on PATH. Add uv's tool bin directory to PATH, then rerun loaf install."}
	}
	if result := runInstallCommand("serena", []string{"init"}, out); !result.ok {
		return installMcpResult{message: "serena init failed: " + result.message}
	}
	return installMcpResult{ok: true, message: "Serena native CLI installed"}
}

func printSerenaNativeInstructions(out io.Writer) {
	fmt.Fprintf(out, "    %s Serena must be installed natively before Loaf can add MCP entries.\n", ansiYellow("⚠"))
	fmt.Fprintf(out, "    %s uv tool install -p 3.13 serena-agent@latest --prerelease=allow\n", ansiGray("Run:"))
	fmt.Fprintf(out, "    %s serena init\n", ansiGray("Then:"))
}

func parseInstallMcpScope(answer string) string {
	switch {
	case strings.HasPrefix(answer, "g"):
		return "global"
	case strings.HasPrefix(answer, "p"):
		return "project"
	default:
		return "no"
	}
}

func parseInstallMcpTargetSelection(answer string, targets []string) []string {
	if answer == "" || strings.HasPrefix(answer, "a") {
		return targets
	}
	var selected []string
	for _, part := range strings.Split(answer, ",") {
		idx, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		idx--
		if idx >= 0 && idx < len(targets) {
			selected = append(selected, targets[idx])
		}
	}
	if len(selected) == 0 {
		return targets
	}
	return selected
}

func buildInstallMcpStatuses(projectRoot string, targets []string, def installMcpDefinition) map[string]installMcpTargetStatus {
	statuses := map[string]installMcpTargetStatus{}
	for _, target := range targets {
		statuses[target] = detectInstallMcpForTarget(projectRoot, target, def.id)
	}
	return statuses
}

func detectInstallMcpForTarget(projectRoot string, target string, mcpID string) installMcpTargetStatus {
	config, ok := installMcpTargetConfigs[target]
	if !ok {
		return installMcpTargetStatus{}
	}
	if installMcpFileConfigured(resolveInstallMcpPath(config.globalPath, projectRoot, true), mcpID) {
		return installMcpTargetStatus{configured: true, scope: "global"}
	}
	if installMcpFileConfigured(resolveInstallMcpPath(config.projectPath, projectRoot, false), mcpID) {
		return installMcpTargetStatus{configured: true, scope: "project"}
	}
	return installMcpTargetStatus{}
}

func installMcpFileConfigured(path string, mcpID string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(body))
	switch mcpID {
	case "linear":
		return strings.Contains(text, "mcp.linear.app") || strings.Contains(text, "linear")
	case "serena":
		return strings.Contains(text, "serena") || strings.Contains(text, "serena start-mcp-server")
	default:
		return strings.Contains(text, strings.ToLower(mcpID))
	}
}

func formatInstallMcpTargetLine(target string, status installMcpTargetStatus) string {
	if status.configured {
		where := ""
		if status.scope != "" {
			where = " (" + status.scope + ")"
		}
		return fmt.Sprintf("    %s %s%s", ansiGreen("✓"), target, where)
	}
	return fmt.Sprintf("    %s %s: not configured", ansiYellow("⚡"), target)
}

func installMcpDoneForTargets(statuses map[string]installMcpTargetStatus, targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !statuses[target].configured {
			return false
		}
	}
	return true
}

func unconfiguredInstallMcpTargets(targets []string, statuses map[string]installMcpTargetStatus) []string {
	var result []string
	for _, target := range targets {
		if !statuses[target].configured {
			result = append(result, target)
		}
	}
	return result
}

type installMcpResult struct {
	ok      bool
	message string
}

func installMcpForTarget(target string, def installMcpDefinition, scope string, projectRoot string) installMcpResult {
	config, ok := installMcpTargetConfigs[target]
	if !ok {
		return installMcpResult{message: "unknown target"}
	}
	args := installMcpArgs(def, target)
	switch target {
	case "claude-code":
		return installMcpViaCLI("claude", append([]string{"mcp", "add", "--scope", installClaudeMcpScope(scope), def.id, "--"}, args...), scope+" scope")
	case "codex":
		return installMcpViaCLI("codex", append([]string{"mcp", "add", def.id, "--"}, args...), scope+" scope (via CLI)")
	case "opencode":
		path := installMcpConfigPath(config, scope, projectRoot)
		if err := mergeOpenCodeMcpConfig(path, def.id, args); err != nil {
			return installMcpResult{message: err.Error()}
		}
		return installMcpResult{ok: true, message: "merged into " + installMcpDisplayPath(config, scope)}
	default:
		if config.format != "json" {
			return installMcpResult{message: "manual configuration required"}
		}
		path := installMcpConfigPath(config, scope, projectRoot)
		if err := mergeJSONMcpConfig(path, config.mcpKey, def.id, args); err != nil {
			return installMcpResult{message: err.Error()}
		}
		return installMcpResult{ok: true, message: "merged into " + installMcpDisplayPath(config, scope)}
	}
}

func installMcpViaCLI(command string, args []string, success string) installMcpResult {
	if !installCommandExists(command) {
		return installMcpResult{message: command + " CLI not found"}
	}
	if result := runInstallCommand(command, args, nil); !result.ok {
		return result
	}
	return installMcpResult{ok: true, message: success}
}

func runInstallCommand(command string, args []string, out io.Writer) installMcpResult {
	cmd := exec.Command(command, args...)
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return installMcpResult{message: err.Error()}
		}
		return installMcpResult{ok: true}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return installMcpResult{message: message}
	}
	return installMcpResult{ok: true}
}

func installClaudeMcpScope(scope string) string {
	if scope == "global" {
		return "user"
	}
	return "project"
}

func installMcpArgs(def installMcpDefinition, target string) []string {
	if def.targetArgs != nil {
		if args, ok := def.targetArgs[target]; ok {
			return args
		}
	}
	return def.defaultArgs
}

func installMcpConfigPath(config installMcpTargetConfig, scope string, projectRoot string) string {
	if scope == "global" {
		return resolveInstallMcpPath(config.globalPath, projectRoot, true)
	}
	return resolveInstallMcpPath(config.projectPath, projectRoot, false)
}

func installMcpDisplayPath(config installMcpTargetConfig, scope string) string {
	if scope == "global" {
		return config.globalPath
	}
	return config.projectPath
}

func resolveInstallMcpPath(path string, projectRoot string, global bool) string {
	if global {
		if strings.HasPrefix(path, "~/") {
			return filepath.Join(installHome(), path[2:])
		}
		return path
	}
	return filepath.Join(projectRoot, filepath.FromSlash(path))
}

func mergeJSONMcpConfig(path string, mcpKey string, serverID string, args []string) error {
	data := map[string]any{}
	if body, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(body, &data); err != nil {
			data = map[string]any{}
		}
	}
	servers := ensureJSONMcpMap(data, mcpKey)
	servers[serverID] = map[string]any{
		"command": args[0],
		"args":    args[1:],
	}
	return writeInstallJSON(path, data)
}

func mergeOpenCodeMcpConfig(path string, serverID string, args []string) error {
	data := map[string]any{}
	if body, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(body, &data); err != nil {
			data = map[string]any{}
		}
	}
	mcp, ok := data["mcp"].(map[string]any)
	if !ok {
		mcp = map[string]any{}
		data["mcp"] = mcp
	}
	mcp[serverID] = map[string]any{
		"type":    "local",
		"command": args,
		"enabled": true,
	}
	return writeInstallJSON(path, data)
}

func ensureJSONMcpMap(data map[string]any, mcpKey string) map[string]any {
	if value, ok := data[mcpKey].(map[string]any); ok {
		return value
	}
	if !strings.Contains(mcpKey, ".") {
		servers := map[string]any{}
		data[mcpKey] = servers
		return servers
	}
	current := data
	parts := strings.Split(mcpKey, ".")
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	last := parts[len(parts)-1]
	servers, ok := current[last].(map[string]any)
	if !ok {
		servers = map[string]any{}
		current[last] = servers
	}
	return servers
}

func writeInstallJSON(path string, data map[string]any) error {
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func readInstallLoafConfig(projectRoot string) map[string]any {
	body, err := os.ReadFile(filepath.Join(projectRoot, ".agents", "loaf.json"))
	if err != nil {
		return map[string]any{}
	}
	var config map[string]any
	if err := json.Unmarshal(body, &config); err != nil {
		return map[string]any{}
	}
	if config == nil {
		return map[string]any{}
	}
	return config
}

func mergeInstallLoafConfigIntegrations(projectRoot string, updates map[string]bool, preserveExisting bool) error {
	config := readInstallLoafConfig(projectRoot)
	integrations, ok := config["integrations"].(map[string]any)
	if !ok {
		integrations = map[string]any{}
		config["integrations"] = integrations
	}
	for id, enabled := range updates {
		if preserveExisting {
			if _, exists := integrations[id]; exists {
				continue
			}
		}
		integrations[id] = map[string]any{"enabled": enabled}
	}
	return writeInstallJSON(filepath.Join(projectRoot, ".agents", "loaf.json"), config)
}

func uniqueInstallTargets(targets []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, target := range targets {
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		result = append(result, target)
	}
	sort.Strings(result)
	return result
}
