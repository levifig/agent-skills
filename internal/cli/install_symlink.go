package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
				Message: fmt.Sprintf("%s exists as a real file; skipped in non-interactive mode (fenced sections may drift from canonical AGENTS.md)", description),
			}
		}
		if options.Prompt != nil {
			approved = options.Prompt(fmt.Sprintf("  %s exists as a regular file. Merge its content into canonical AGENTS.md, back it up as %s.bak, and replace with a symlink? [y/N] ", description, description))
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
	wantRootAgents := needsRootInstallAgentsFile(selectedTargets)
	if !wantClaude && !wantRootAgents {
		return results
	}

	canonical := filepath.Join(projectRoot, "AGENTS.md")
	rootResult := ensureRootInstallAgentsFile(projectRoot, options)
	results["./AGENTS.md"] = rootResult
	if rootResult.Error != "" {
		return results
	}
	options.CanonicalPath = canonical
	options.ProjectRoot = projectRoot

	if wantClaude {
		linkPath := filepath.Join(projectRoot, ".claude", "CLAUDE.md")
		relTarget := relativeInstallLinkTarget(linkPath, canonical)
		results[".claude/CLAUDE.md"] = ensureInstallSymlink(linkPath, relTarget, ".claude/CLAUDE.md", options)
	}
	return results
}

func needsRootInstallAgentsFile(targets []string) bool {
	for _, target := range targets {
		if agentsMDInstallTargets[target] {
			return true
		}
	}
	return false
}

func ensureRootInstallAgentsFile(projectRoot string, options installSymlinkOptions) installSymlinkResult {
	canonical := filepath.Join(projectRoot, "AGENTS.md")
	legacy := filepath.Join(projectRoot, ".agents", "AGENTS.md")
	legacyExists := installFileExists(legacy) && !installIsSymlink(legacy)
	if installIsDirectory(canonical) {
		err := fmt.Errorf("./AGENTS.md is a directory; expected a canonical real file")
		return installSymlinkError("error", err.Error(), err)
	}

	if installIsSymlink(canonical) && legacyExists && installSymlinkPointsTo(canonical, legacy) {
		linkBackup := collisionSafeInstallBackupPath(canonical)
		if err := os.Rename(canonical, linkBackup); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to migrate ./AGENTS.md: %v", err), err)
		}
		if err := os.Rename(legacy, canonical); err != nil {
			_ = os.Rename(linkBackup, canonical)
			return installSymlinkError("error", fmt.Sprintf("Failed to migrate .agents/AGENTS.md: %v", err), err)
		}
		_ = os.Remove(linkBackup)
		return installSymlinkResult{Action: "migrated", Message: "Migrated .agents/AGENTS.md to canonical ./AGENTS.md"}
	}

	if !installPathExists(canonical) {
		if legacyExists {
			if err := os.Rename(legacy, canonical); err != nil {
				return installSymlinkError("error", fmt.Sprintf("Failed to migrate .agents/AGENTS.md: %v", err), err)
			}
			return installSymlinkResult{Action: "migrated", Message: "Migrated .agents/AGENTS.md to canonical ./AGENTS.md"}
		}
		if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to create ./AGENTS.md: %v", err), err)
		}
		return installSymlinkResult{Action: "created", Message: "Created canonical ./AGENTS.md"}
	}

	if installIsSymlink(canonical) {
		approved := options.AssumeYes
		if !approved {
			if options.NonInteractive {
				return installSymlinkResult{Action: "skipped-no-tty", Message: "./AGENTS.md is a symlink; skipped conversion to a canonical real file in non-interactive mode"}
			}
			if options.Prompt != nil {
				approved = options.Prompt("  ./AGENTS.md is a symlink. Preserve its current content and replace it with the canonical real file? [y/N] ")
			}
		}
		if !approved {
			return installSymlinkResult{Action: "declined-replace", Message: "Left ./AGENTS.md as a symlink"}
		}
		body, err := os.ReadFile(canonical)
		if err != nil && !os.IsNotExist(err) {
			return installSymlinkError("error", fmt.Sprintf("Failed to read ./AGENTS.md: %v", err), err)
		}
		backup := collisionSafeInstallBackupPath(canonical)
		if err := os.Rename(canonical, backup); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to back up ./AGENTS.md: %v", err), err)
		}
		if err := os.WriteFile(canonical, body, 0o644); err != nil {
			_ = os.Rename(backup, canonical)
			return installSymlinkError("error", fmt.Sprintf("Failed to create canonical ./AGENTS.md: %v", err), err)
		}
		return installSymlinkResult{Action: "replaced-file", Message: "Backed up the old ./AGENTS.md symlink and created a canonical real file", BackupPath: backup}
	}

	if legacyExists {
		approved := options.AssumeYes
		if !approved {
			if options.NonInteractive {
				return installSymlinkResult{Action: "skipped-no-tty", Message: "Both ./AGENTS.md and .agents/AGENTS.md are real files; skipped merge in non-interactive mode"}
			}
			if options.Prompt != nil {
				approved = options.Prompt("  Both ./AGENTS.md and .agents/AGENTS.md are real files. Merge legacy user content into root AGENTS.md and retire the legacy file with a backup? [y/N] ")
			}
		}
		if !approved {
			return installSymlinkResult{Action: "declined-replace", Message: "Left both ./AGENTS.md and .agents/AGENTS.md unchanged"}
		}
		body, err := os.ReadFile(legacy)
		if err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to migrate .agents/AGENTS.md: %v", err), err)
		}
		stripped := stripDoctorLoafFence(string(body))
		backup := collisionSafeInstallBackupPath(legacy)
		if err := os.Rename(legacy, backup); err != nil {
			return installSymlinkError("error", fmt.Sprintf("Failed to back up .agents/AGENTS.md: %v", err), err)
		}
		merged, err := mergeLegacyAgentsContentIntoCanonical(canonical, stripped, ".agents/AGENTS.md")
		if err != nil {
			if rollbackErr := os.Rename(backup, legacy); rollbackErr != nil {
				err = fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
			return installSymlinkError("error", fmt.Sprintf("Failed to migrate .agents/AGENTS.md: %v", err), err)
		}
		return installSymlinkResult{Action: "migrated", Message: "Migrated legacy .agents/AGENTS.md into canonical ./AGENTS.md", BackupPath: backup, Merged: merged}
	}

	return installSymlinkResult{Action: "already-correct", Message: "Canonical ./AGENTS.md already exists"}
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

func installIsDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func collisionSafeInstallBackupPath(path string) string {
	base := path + ".bak"
	if !installPathExists(base) {
		return base
	}
	for index := 1; ; index++ {
		candidate := base + "." + strconv.Itoa(index)
		if !installPathExists(candidate) {
			return candidate
		}
	}
}

func mergeLegacyAgentsContentIntoCanonical(canonical string, stripped string, relSource string) (bool, error) {
	if stripped == "" {
		return false, nil
	}
	existing, err := os.ReadFile(canonical)
	if err != nil {
		return false, err
	}
	trimmedExisting := strings.TrimRight(string(existing), " \t\r\n")
	merged := trimmedExisting + "\n\n## Migrated from " + relSource + "\n\n" + stripped + "\n"
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(canonical); statErr == nil {
		mode = info.Mode().Perm()
	}
	temp, err := os.CreateTemp(filepath.Dir(canonical), ".loaf-agents-md-*")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return false, err
	}
	if _, err := temp.WriteString(merged); err != nil {
		_ = temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(tempPath, canonical); err != nil {
		return false, err
	}
	return true, nil
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
