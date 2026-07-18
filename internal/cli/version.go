package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdlibRuntime "runtime"
	"strings"
)

type builtTarget struct {
	name string
	path string
}

var versionTargetOutputs = []builtTarget{
	{name: "claude-code", path: "plugins/loaf/"},
	{name: "cursor", path: "dist/cursor/"},
	{name: "opencode", path: "dist/opencode/"},
	{name: "codex", path: "dist/codex/"},
}

func (r Runner) runVersion(out io.Writer) error {
	root, rootErr := r.resolveInstalledDistributionRoot()
	if rootErr != nil {
		root = ""
	}
	version := packageVersion(root)

	fmt.Fprintf(out, "\n%s %s%s\n", ansiBold("loaf"), version, buildInfoSuffix(r.BuildCommit, r.BuildDate))
	fmt.Fprintf(out, "%s %s\n", ansiGray("go"), strings.TrimPrefix(runtimeVersion(), "go"))

	// Targets and Content describe the installed distribution. Without
	// resolvable executable provenance there is no distribution to describe —
	// an empty root would make the lookups below relative to the working
	// directory, counting whatever checkout the caller happens to stand in —
	// so the degraded output ends at identity plus the resolver's guidance.
	if rootErr != nil {
		fmt.Fprintf(out, "\n%s\n\n", ansiGray(rootErr.Error()))
		return nil
	}

	targets := builtTargets(root)
	if len(targets) > 0 {
		fmt.Fprintf(out, "\n%s\n", ansiBold("Targets:"))
		maxName := 0
		for _, target := range targets {
			if len(target.name) > maxName {
				maxName = len(target.name)
			}
		}
		for _, target := range targets {
			fmt.Fprintf(out, "  %-*s%s\n", maxName+2, target.name, ansiGray(target.path))
		}
	}

	fmt.Fprintf(out, "\n%s\n", ansiBold("Content:"))
	fmt.Fprintf(out, "  Skills:  %d\n", countSkillDirs(root))
	fmt.Fprintf(out, "  Agents:  %d\n", countAgentFiles(root))
	fmt.Fprintf(out, "  Hooks:   %d\n\n", countHookEntries(root))
	return nil
}

// buildInfoSuffix renders optional build metadata for the version line. The
// semver identifier is build-independent, so commit/date are appended as a
// parenthetical suffix only when supplied (release builds). It returns an empty
// string when neither is set, preserving the clean `loaf <version>` output.
func buildInfoSuffix(commit, date string) string {
	commit = strings.TrimSpace(commit)
	date = strings.TrimSpace(date)
	var parts []string
	if date != "" {
		parts = append(parts, "built "+date)
	}
	if commit != "" {
		parts = append(parts, "git "+commit)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, " · ") + ")"
}

func findLoafPackageRoot(path string, seen map[string]bool) (string, bool) {
	if path == "" {
		return "", false
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	for {
		if !seen[clean] {
			seen[clean] = true
			if packageName(clean) == "loaf" {
				return clean, true
			}
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return "", false
		}
		clean = parent
	}
}

func packageName(root string) string {
	var pkg struct {
		Name string `json:"name"`
	}
	if err := readPackageJSON(root, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}

func packageVersion(root string) string {
	var pkg struct {
		Version string `json:"version"`
	}
	if err := readPackageJSON(root, &pkg); err != nil || pkg.Version == "" {
		return "0.0.0"
	}
	return pkg.Version
}

func readPackageJSON(root string, target any) error {
	if root == "" {
		return fmt.Errorf("missing root")
	}
	body, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func builtTargets(root string) []builtTarget {
	var targets []builtTarget
	for _, target := range versionTargetOutputs {
		if isDir(filepath.Join(root, filepath.FromSlash(target.path))) {
			targets = append(targets, target)
		}
	}
	return targets
}

func countSkillDirs(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, "content", "skills"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func countAgentFiles(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, "content", "agents"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
	}
	return count
}

func countHookEntries(root string) int {
	body, err := os.ReadFile(filepath.Join(root, "config", "hooks.yaml"))
	if err != nil {
		return 0
	}
	allowedSections := map[string]bool{
		"pre-tool":  true,
		"post-tool": true,
		"session":   true,
	}
	inHooks := false
	activeSection := ""
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			inHooks = trimmed == "hooks:"
			activeSection = ""
			continue
		}
		if !inHooks {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			section := strings.TrimSuffix(trimmed, ":")
			if allowedSections[section] {
				activeSection = section
			} else {
				activeSection = ""
			}
			continue
		}
		if activeSection != "" && strings.HasPrefix(strings.TrimLeft(line, " "), "- ") {
			count++
		}
	}
	return count
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ansiBold(value string) string {
	return "\x1b[1m" + value + "\x1b[0m"
}

func ansiGray(value string) string {
	return "\x1b[90m" + value + "\x1b[0m"
}

func runtimeVersion() string {
	return strings.TrimPrefix(stdlibRuntime.Version(), "go")
}
