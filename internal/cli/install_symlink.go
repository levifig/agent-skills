package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

type installSymlinkOptions struct {
	Prompt         func(question string) bool
	NonInteractive bool
	AssumeYes      bool
	CanonicalPath  string
	ProjectRoot    string
}

type installSymlinkResult struct {
	Action     string
	Message    string
	BackupPath string
	Merged     bool
	Error      string
}

var agentsMDInstallTargets = map[string]bool{
	"cursor":   true,
	"codex":    true,
	"opencode": true,
	"amp":      true,
	"gemini":   true,
}

func ensureInstallSymlink(linkPath string, relativeTarget string, description string, options installSymlinkOptions) installSymlinkResult {
	expectedAbs := filepath.Clean(filepath.Join(filepath.Dir(linkPath), relativeTarget))
	if !installPathExists(linkPath) {
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to create %s: %v", description, err), err)
		}
		if err := os.Symlink(relativeTarget, linkPath); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to create %s: %v", description, err), err)
		}
		return installSymlinkResult{Action: "created", Message: fmt.Sprintf("Created %s -> %s", description, relativeTarget)}
	}

	if installIsSymlink(linkPath) {
		if installSymlinkPointsTo(linkPath, expectedAbs) {
			return installSymlinkResult{Action: "already-correct", Message: fmt.Sprintf("%s already points to %s", description, relativeTarget)}
		}
		actualTarget := resolveInstallSymlinkTarget(linkPath)
		if actualTarget == "" {
			actualTarget = "<unreadable>"
		}
		approved := options.AssumeYes
		if !approved {
			if options.NonInteractive {
				return installSymlinkResult{
					Action:  "skipped-no-tty",
					Message: fmt.Sprintf("%s points to the wrong target (%s); skipped in non-interactive mode", description, actualTarget),
				}
			}
			if options.Prompt != nil {
				approved = options.Prompt(fmt.Sprintf("  %s points to %s, not %s. Relink? [y/N] ", description, actualTarget, relativeTarget))
			}
		}
		if !approved {
			return installSymlinkResult{Action: "declined-relink", Message: fmt.Sprintf("Left %s pointing at %s", description, actualTarget)}
		}
		if err := os.Remove(linkPath); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to relink %s: %v", description, err), err)
		}
		if err := os.Symlink(relativeTarget, linkPath); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to relink %s: %v", description, err), err)
		}
		return installSymlinkResult{Action: "relinked", Message: fmt.Sprintf("Relinked %s -> %s", description, relativeTarget)}
	}

	approved := options.AssumeYes
	if !approved {
		if options.NonInteractive {
			return installSymlinkResult{
				Action:  "skipped-no-tty",
				Message: fmt.Sprintf("%s exists as a real file; skipped in non-interactive mode (fenced sections may drift between it and .agents/AGENTS.md)", description),
			}
		}
		if options.Prompt != nil {
			approved = options.Prompt(fmt.Sprintf("  %s exists as a regular file. Merge its content into .agents/AGENTS.md, back it up as %s.bak, and replace with a symlink? [y/N] ", description, description))
		}
	}
	if !approved {
		return installSymlinkResult{Action: "declined-replace", Message: fmt.Sprintf("Left %s as a regular file (fenced sections may drift)", description)}
	}

	sourceContent, err := os.ReadFile(linkPath)
	if err != nil {
		return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
	}
	stripped := stripDoctorLoafFence(string(sourceContent))
	merged := false
	if options.CanonicalPath != "" && stripped != "" {
		root := options.ProjectRoot
		if root == "" {
			root = filepath.Dir(linkPath)
		}
		relSource, err := filepath.Rel(root, linkPath)
		if err != nil {
			relSource = linkPath
		}
		merged, err = mergeDoctorContentIntoCanonical(options.CanonicalPath, stripped, relSource)
		if err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
		}
	}

	backupPath := linkPath + ".bak"
	if installPathExists(backupPath) {
		if err := os.RemoveAll(backupPath); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
		}
	}
	if err := os.Rename(linkPath, backupPath); err != nil {
		return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
	}
	if options.CanonicalPath != "" && !installFileExists(options.CanonicalPath) {
		if err := os.MkdirAll(filepath.Dir(options.CanonicalPath), 0o755); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
		}
		if err := os.WriteFile(options.CanonicalPath, []byte{}, 0o644); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
		}
	}
	if err := os.Symlink(relativeTarget, linkPath); err != nil {
		return installSymlinkError("error", fmt.Sprintf("Failed to replace %s: %v", description, err), err)
	}
	suffix := ""
	if merged {
		suffix = " (merged content into canonical)"
	}
	return installSymlinkResult{
		Action:     "replaced-file",
		Message:    fmt.Sprintf("Backed up %s to %s.bak and created symlink -> %s%s", description, description, relativeTarget, suffix),
		BackupPath: backupPath,
		Merged:     merged,
	}
}

func ensureProjectInstallSymlinks(projectRoot string, selectedTargets []string, hasClaudeCode bool, options installSymlinkOptions) map[string]installSymlinkResult {
	results := map[string]installSymlinkResult{}
	wantClaude := hasClaudeCode || containsString(selectedTargets, "claude-code")
	wantRootAgents := needsRootInstallAgentsSymlink(selectedTargets)
	if !wantClaude && !wantRootAgents {
		return results
	}

	canonical := filepath.Join(projectRoot, ".agents", "AGENTS.md")
	if !installFileExists(canonical) {
		if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
			results[".agents/AGENTS.md"] = installSymlinkError("error", fmt.Sprintf("Failed to create .agents/AGENTS.md: %v", err), err)
			return results
		}
		if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
			results[".agents/AGENTS.md"] = installSymlinkError("error", fmt.Sprintf("Failed to create .agents/AGENTS.md: %v", err), err)
			return results
		}
	}
	options.CanonicalPath = canonical
	options.ProjectRoot = projectRoot

	if wantClaude {
		linkPath := filepath.Join(projectRoot, ".claude", "CLAUDE.md")
		relTarget := relativeInstallLinkTarget(linkPath, canonical)
		results[".claude/CLAUDE.md"] = ensureInstallSymlink(linkPath, relTarget, ".claude/CLAUDE.md", options)
	}
	if wantRootAgents {
		linkPath := filepath.Join(projectRoot, "AGENTS.md")
		relTarget := relativeInstallLinkTarget(linkPath, canonical)
		results["./AGENTS.md"] = ensureInstallSymlink(linkPath, relTarget, "./AGENTS.md", options)
	}
	return results
}

func needsRootInstallAgentsSymlink(targets []string) bool {
	for _, target := range targets {
		if agentsMDInstallTargets[target] {
			return true
		}
	}
	return false
}

func relativeInstallLinkTarget(linkPath string, canonicalPath string) string {
	rel, err := filepath.Rel(filepath.Dir(linkPath), canonicalPath)
	if err != nil {
		return canonicalPath
	}
	return rel
}

func installSymlinkError(action string, message string, err error) installSymlinkResult {
	return installSymlinkResult{Action: action, Message: message, Error: err.Error()}
}

func installPathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func installFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func installIsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func resolveInstallSymlinkTarget(linkPath string) string {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return ""
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
}

func installSymlinkPointsTo(linkPath string, expectedAbs string) bool {
	resolved := resolveInstallSymlinkTarget(linkPath)
	return resolved != "" && filepath.Clean(resolved) == filepath.Clean(expectedAbs)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
