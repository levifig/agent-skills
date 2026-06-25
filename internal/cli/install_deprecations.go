package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	installDeprecationManifestPath = "config/deprecations.json"
	defaultDeprecationWindow       = "one-release"
)

type installDeprecationManifest struct {
	Version            int                         `json:"version"`
	RetiredTargets     []retiredInstallTarget      `json:"retired_targets"`
	RetiredSkills      []retiredInstallSkill       `json:"retired_skills"`
	RetiredAgents      []retiredInstallAgent       `json:"retired_agents"`
	ExternalizedSkills []externalizedInstallSkill  `json:"externalized_skills"`
	Relocations        []installRelocationManifest `json:"relocations"`
	Aliases            []installAliasManifest      `json:"aliases"`
}

type retiredInstallTarget struct {
	Target  string   `json:"target"`
	Since   string   `json:"since"`
	Window  string   `json:"window"`
	Reason  string   `json:"reason"`
	Signoff string   `json:"signoff"`
	Paths   []string `json:"paths"`
}

type retiredInstallSkill struct {
	Skill      string   `json:"skill"`
	Since      string   `json:"since"`
	Window     string   `json:"window"`
	Reason     string   `json:"reason"`
	Signoff    string   `json:"signoff"`
	SkillHomes []string `json:"skill_homes"`
}

type retiredInstallAgent struct {
	Agent      string   `json:"agent"`
	Since      string   `json:"since"`
	Window     string   `json:"window"`
	Reason     string   `json:"reason"`
	Signoff    string   `json:"signoff"`
	AgentHomes []string `json:"agent_homes"`
}

type externalizedInstallSkill struct {
	Skill          string   `json:"skill"`
	Since          string   `json:"since"`
	Window         string   `json:"window"`
	Reason         string   `json:"reason"`
	Signoff        string   `json:"signoff"`
	Source         string   `json:"source"`
	InstallCommand string   `json:"install_command"`
	SkillHomes     []string `json:"skill_homes"`
}

type installRelocationManifest struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	OwnerMarker string `json:"owner_marker"`
	Since       string `json:"since"`
	Window      string `json:"window"`
	Reason      string `json:"reason"`
	Signoff     string `json:"signoff"`
}

type installAliasManifest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Since   string `json:"since"`
	Window  string `json:"window"`
	Reason  string `json:"reason"`
	Signoff string `json:"signoff"`
}

type installDeprecationCleanupResult struct {
	Removed      []installDeprecationCleanupAction
	Externalized []installDeprecationCleanupAction
	Aliases      []installDeprecationCleanupAction
	Skipped      []installDeprecationCleanupAction
}

type installDeprecationCleanupAction struct {
	Kind    string
	Name    string
	Path    string
	Reason  string
	Since   string
	Window  string
	Signoff string
	Source  string
	Command string
	Action  string
}

func runInstallDeprecationCleanup(loafRoot string, out io.Writer, allowDestructive bool) error {
	manifest, found, err := loadInstallDeprecationManifest(loafRoot)
	if err != nil {
		return err
	}
	if !found || manifest.isEmpty() {
		return nil
	}
	result, err := applyInstallDeprecationCleanup(manifest, installPathContext(), allowDestructive)
	if err != nil {
		return err
	}
	writeInstallDeprecationCleanup(out, result)
	return nil
}

func loadInstallDeprecationManifest(loafRoot string) (installDeprecationManifest, bool, error) {
	path := filepath.Join(loafRoot, installDeprecationManifestPath)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return installDeprecationManifest{}, false, nil
		}
		return installDeprecationManifest{}, false, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest installDeprecationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return installDeprecationManifest{}, true, fmt.Errorf("read install deprecation manifest: %w", err)
	}
	if manifest.Version != 1 {
		return installDeprecationManifest{}, true, fmt.Errorf("install deprecation manifest version %d is not supported", manifest.Version)
	}
	return manifest, true, nil
}

func (m installDeprecationManifest) isEmpty() bool {
	return len(m.RetiredTargets) == 0 &&
		len(m.RetiredSkills) == 0 &&
		len(m.RetiredAgents) == 0 &&
		len(m.ExternalizedSkills) == 0 &&
		len(m.Relocations) == 0 &&
		len(m.Aliases) == 0
}

func applyInstallDeprecationCleanup(manifest installDeprecationManifest, pathContext map[string]string, allowDestructive bool) (installDeprecationCleanupResult, error) {
	var result installDeprecationCleanupResult
	for _, target := range manifest.RetiredTargets {
		for _, rawPath := range target.Paths {
			path, err := expandInstallDeprecationPath(rawPath, pathContext)
			if err != nil {
				return result, err
			}
			action := installDeprecationCleanupAction{
				Kind:    "target",
				Name:    target.Target,
				Path:    path,
				Reason:  target.Reason,
				Since:   target.Since,
				Window:  deprecationWindow(target.Window),
				Signoff: target.Signoff,
			}
			if !dirExistsForInstall(path) {
				action.Action = "missing"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !fileExistsForInstall(filepath.Join(path, loafInstallMarkerFile)) {
				action.Action = "unmarked"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !allowDestructive {
				action.Action = "confirmation-required"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				return result, err
			}
			action.Action = "removed"
			result.Removed = append(result.Removed, action)
		}
	}
	for _, skill := range manifest.RetiredSkills {
		for _, rawHome := range skill.SkillHomes {
			home, err := expandInstallDeprecationPath(rawHome, pathContext)
			if err != nil {
				return result, err
			}
			path := filepath.Join(home, skill.Skill)
			action := installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    skill.Skill,
				Path:    path,
				Reason:  skill.Reason,
				Since:   skill.Since,
				Window:  deprecationWindow(skill.Window),
				Signoff: skill.Signoff,
			}
			if !dirExistsForInstall(path) {
				action.Action = "missing"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !fileExistsForInstall(filepath.Join(path, "SKILL.md")) {
				action.Action = "unmarked"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !allowDestructive {
				action.Action = "confirmation-required"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				return result, err
			}
			action.Action = "removed"
			result.Removed = append(result.Removed, action)
		}
	}
	for _, agent := range manifest.RetiredAgents {
		for _, rawHome := range agent.AgentHomes {
			home, err := expandInstallDeprecationPath(rawHome, pathContext)
			if err != nil {
				return result, err
			}
			path := filepath.Join(home, agent.Agent+".md")
			action := installDeprecationCleanupAction{
				Kind:    "agent",
				Name:    agent.Agent,
				Path:    path,
				Reason:  agent.Reason,
				Since:   agent.Since,
				Window:  deprecationWindow(agent.Window),
				Signoff: agent.Signoff,
			}
			if !fileExistsForInstall(path) {
				action.Action = "missing"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !isLoafOwnedAgentFile(home) {
				action.Action = "unmarked"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if !allowDestructive {
				action.Action = "confirmation-required"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			if err := os.Remove(path); err != nil {
				return result, err
			}
			action.Action = "removed"
			result.Removed = append(result.Removed, action)
		}
	}
	for _, skill := range manifest.ExternalizedSkills {
		for _, rawHome := range skill.SkillHomes {
			home, err := expandInstallDeprecationPath(rawHome, pathContext)
			if err != nil {
				return result, err
			}
			path := filepath.Join(home, skill.Skill)
			if !dirExistsForInstall(path) || !fileExistsForInstall(filepath.Join(path, "SKILL.md")) {
				continue
			}
			result.Externalized = append(result.Externalized, installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    skill.Skill,
				Path:    path,
				Reason:  skill.Reason,
				Since:   skill.Since,
				Window:  deprecationWindow(skill.Window),
				Signoff: skill.Signoff,
				Source:  skill.Source,
				Command: skill.InstallCommand,
				Action:  "externalized",
			})
		}
	}
	for _, relocation := range manifest.Relocations {
		from, err := expandInstallDeprecationPath(relocation.From, pathContext)
		if err != nil {
			return result, err
		}
		to, err := expandInstallDeprecationPath(relocation.To, pathContext)
		if err != nil {
			return result, err
		}
		ownerMarker := ""
		if relocation.OwnerMarker != "" {
			ownerMarker, err = expandInstallDeprecationPath(relocation.OwnerMarker, pathContext)
			if err != nil {
				return result, err
			}
		}
		action := installDeprecationCleanupAction{
			Kind:    "path",
			Name:    relocation.ID,
			Path:    from + " -> " + to,
			Reason:  relocation.Reason,
			Since:   relocation.Since,
			Window:  deprecationWindow(relocation.Window),
			Signoff: relocation.Signoff,
		}
		if !dirExistsForInstall(from) {
			action.Action = "missing"
			result.Skipped = append(result.Skipped, action)
			continue
		}
		if !isLoafOwnedRelocationDir(from, ownerMarker) {
			action.Action = "unmarked"
			result.Skipped = append(result.Skipped, action)
			continue
		}
		if !allowDestructive {
			action.Action = "confirmation-required"
			result.Skipped = append(result.Skipped, action)
			continue
		}
		if dirExistsForInstall(to) {
			if err := os.RemoveAll(from); err != nil {
				return result, err
			}
			action.Action = "removed-stale"
			result.Removed = append(result.Removed, action)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return result, err
		}
		if err := os.Rename(from, to); err != nil {
			return result, err
		}
		action.Action = "relocated"
		result.Removed = append(result.Removed, action)
	}
	for _, alias := range manifest.Aliases {
		result.Aliases = append(result.Aliases, installDeprecationCleanupAction{
			Kind:    "alias",
			Name:    alias.From,
			Path:    alias.To,
			Reason:  alias.Reason,
			Since:   alias.Since,
			Window:  deprecationWindow(alias.Window),
			Signoff: alias.Signoff,
			Action:  "alias",
		})
	}
	return result, nil
}

func deprecationWindow(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultDeprecationWindow
	}
	return value
}

func isLoafOwnedInstallDir(path string) bool {
	return fileExistsForInstall(filepath.Join(path, loafInstallMarkerFile)) ||
		fileExistsForInstall(filepath.Join(filepath.Dir(path), loafInstallMarkerFile)) ||
		fileExistsForInstall(filepath.Join(path, "SKILL.md"))
}

func isLoafOwnedRelocationDir(path string, ownerMarker string) bool {
	return isLoafOwnedInstallDir(path) || (ownerMarker != "" && fileExistsForInstall(ownerMarker))
}

func isLoafOwnedAgentFile(agentHome string) bool {
	return fileExistsForInstall(filepath.Join(agentHome, loafInstallMarkerFile)) ||
		fileExistsForInstall(filepath.Join(filepath.Dir(agentHome), loafInstallMarkerFile))
}

func installPathContext() map[string]string {
	home := installHome()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	return map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": xdgConfig,
	}
}

func expandInstallDeprecationPath(path string, context map[string]string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("install deprecation path cannot be empty")
	}
	expanded := path
	if strings.HasPrefix(expanded, "~/") {
		home := context["HOME"]
		if home == "" {
			return "", fmt.Errorf("cannot expand %q without HOME", path)
		}
		expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~/"))
	}
	for key, value := range context {
		if value == "" {
			continue
		}
		expanded = strings.ReplaceAll(expanded, "${"+key+"}", value)
		expanded = strings.ReplaceAll(expanded, "$"+key, value)
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("install deprecation path %q must expand to an absolute path", path)
	}
	return filepath.Clean(expanded), nil
}

func writeInstallDeprecationCleanup(out io.Writer, result installDeprecationCleanupResult) {
	if len(result.Removed) == 0 && len(result.Externalized) == 0 && len(result.Aliases) == 0 && len(result.Skipped) == 0 {
		return
	}
	fmt.Fprintf(out, "  %s install deprecation cleanup\n", ansiGray("•"))
	for _, action := range result.Removed {
		switch action.Action {
		case "relocated":
			fmt.Fprintf(out, "    %s relocated %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		case "removed-stale":
			fmt.Fprintf(out, "    %s removed stale relocated %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		default:
			fmt.Fprintf(out, "    %s removed retired %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		}
		writeInstallDeprecationMetadata(out, action)
		fmt.Fprintln(out)
	}
	for _, action := range result.Externalized {
		fmt.Fprintf(out, "    %s externalized %s %s at %s", ansiGray("-"), action.Kind, action.Name, ansiGray(action.Path))
		writeInstallDeprecationMetadata(out, action)
		if action.Source != "" {
			fmt.Fprintf(out, " source: %s", action.Source)
		}
		if action.Command != "" {
			fmt.Fprintf(out, " command: %s", action.Command)
		}
		fmt.Fprintln(out)
	}
	for _, action := range result.Aliases {
		fmt.Fprintf(out, "    %s alias %s -> %s", ansiGray("-"), action.Name, action.Path)
		writeInstallDeprecationMetadata(out, action)
		fmt.Fprintln(out)
	}
	for _, action := range result.Skipped {
		switch action.Action {
		case "missing":
			fmt.Fprintf(out, "    %s retired %s %s already absent at %s\n", ansiGray("-"), action.Kind, action.Name, ansiGray(action.Path))
		case "unmarked":
			fmt.Fprintf(out, "    %s skipped retired %s %s at %s; path is not marked as Loaf-owned\n", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		case "confirmation-required":
			fmt.Fprintf(out, "    %s skipped %s %s at %s; rerun with --yes to apply destructive deprecation cleanup\n", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		}
	}
}

func writeInstallDeprecationMetadata(out io.Writer, action installDeprecationCleanupAction) {
	if action.Reason != "" {
		fmt.Fprintf(out, " — %s", action.Reason)
	}
	if action.Since != "" || action.Window != "" {
		fmt.Fprintf(out, " (since %s, window %s)", emptyInstallDeprecationField(action.Since), emptyInstallDeprecationField(action.Window))
	}
	if action.Signoff != "" {
		fmt.Fprintf(out, " [signoff: %s]", action.Signoff)
	}
}

func emptyInstallDeprecationField(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unspecified"
	}
	return value
}
