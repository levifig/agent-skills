package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	fencedStartMarker = "<!-- loaf:managed:start"
	fencedEndMarker   = "<!-- loaf:managed:end -->"
	fencedWarning     = "<!-- Maintained by loaf install/upgrade - do not edit manually -->"
)

var fencedTargetFiles = map[string]string{
	"claude-code": ".claude/CLAUDE.md",
	"cursor":      ".agents/AGENTS.md",
	"codex":       ".agents/AGENTS.md",
	"opencode":    ".agents/AGENTS.md",
	"amp":         ".agents/AGENTS.md",
}

type fencedSectionRange struct {
	start   int
	end     int
	version string
}

type fencedInstallResult struct {
	Action  string
	Version string
	Error   string
}

func installFencedSection(targetFile string, version string, upgrade bool) (fencedInstallResult, error) {
	if version == "" {
		version = "0.0.0"
	}
	body, err := os.ReadFile(targetFile)
	fileExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fencedInstallResult{}, err
	}
	content := string(body)
	section, hasSection := findFencedSectionRange(content)
	newContent := generateFencedContent(version)

	switch {
	case hasSection && upgrade && section.version == version:
		return fencedInstallResult{Action: "skipped", Version: version}, nil
	case hasSection:
		before := strings.TrimRight(content[:section.start], " \t\r\n")
		after := strings.TrimLeft(content[section.end:], " \t\r\n")
		updated := before
		if updated != "" {
			updated += "\n\n"
		}
		updated += newContent
		if after != "" {
			updated += "\n\n" + after
		} else {
			updated += "\n"
		}
		if err := os.WriteFile(targetFile, []byte(updated), 0o644); err != nil {
			return fencedInstallResult{}, err
		}
		return fencedInstallResult{Action: "updated", Version: version}, nil
	case fileExisted:
		trimmed := strings.TrimRight(content, " \t\r\n")
		updated := trimmed
		if updated != "" {
			updated += "\n\n"
		}
		updated += newContent + "\n"
		if err := os.WriteFile(targetFile, []byte(updated), 0o644); err != nil {
			return fencedInstallResult{}, err
		}
		return fencedInstallResult{Action: "appended", Version: version}, nil
	default:
		if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
			return fencedInstallResult{}, err
		}
		if err := os.WriteFile(targetFile, []byte(newContent+"\n"), 0o644); err != nil {
			return fencedInstallResult{}, err
		}
		return fencedInstallResult{Action: "created", Version: version}, nil
	}
}

func installFencedSectionsForTargets(targets []string, projectRoot string, version string, upgrade bool) map[string]fencedInstallResult {
	results := map[string]fencedInstallResult{}
	writtenPaths := map[string]string{}
	for _, target := range targets {
		relPath, ok := fencedTargetFiles[target]
		if !ok {
			results[target] = fencedInstallResult{Action: "error", Error: fmt.Sprintf("Unknown target: %s", target)}
			continue
		}
		targetFile := filepath.Join(projectRoot, filepath.FromSlash(relPath))
		canonicalBefore := canonicalInstallPath(targetFile)
		if writtenVersion, ok := writtenPaths[canonicalBefore]; ok {
			results[target] = fencedInstallResult{Action: "skipped", Version: writtenVersion}
			continue
		}
		result, err := installFencedSection(targetFile, version, upgrade)
		if err != nil {
			results[target] = fencedInstallResult{Action: "error", Error: err.Error()}
			continue
		}
		results[target] = result
		writtenPaths[canonicalInstallPath(targetFile)] = result.Version
	}
	return results
}

func findFencedSectionRange(content string) (fencedSectionRange, bool) {
	start := strings.Index(content, fencedStartMarker)
	if start < 0 {
		return fencedSectionRange{}, false
	}
	endStart := strings.Index(content[start:], fencedEndMarker)
	if endStart < 0 {
		return fencedSectionRange{}, false
	}
	end := start + endStart + len(fencedEndMarker)
	startLineEnd := strings.Index(content[start:], "-->")
	version := ""
	if startLineEnd >= 0 {
		startLine := content[start : start+startLineEnd+3]
		if match := regexp.MustCompile(`v([\d.]+(?:-[-\w.]+)?)`).FindStringSubmatch(startLine); len(match) == 2 {
			version = match[1]
		}
	}
	return fencedSectionRange{start: start, end: end, version: version}, true
}

func generateFencedContent(version string) string {
	return strings.Join([]string{
		"<!-- loaf:managed:start v" + version + " -->",
		fencedWarning,
		"## Loaf Framework",
		"",
		"**Journal Entry Types:**",
		"- `decision(scope)`: Key decisions with rationale",
		"- `discover(scope)`: Something learned",
		"- `block(scope)` / `unblock(scope)`: Blockers and resolutions",
		"- `spark(scope)`: Ideas to promote via `/idea`",
		"- `todo(scope)`: Action items to promote to tasks",
		"",
		"**CLI Commands:**",
		"- `loaf journal log/recent/search/context` - Project journal",
		"- `loaf check` - Run enforcement hooks",
		"- `loaf task/spec/kb` - Task and knowledge management",
		"",
		"**Journal Discipline:**",
		"Before completing any response that includes edits, commits, or significant decisions, log journal entries using `loaf journal log \"type(scope): description\"`. Entry types: `decision`, `discover`, `wrap`. Do not defer journaling - log before responding.",
		"In Codex Auto mode, when the user explicitly installed the managed basic-command policy, use the exact path-pinned Loaf executable in the managed `CODEX_HOME/AGENTS.md` block; do not substitute a bare `loaf`. The policy authorizes only explicitly classified basic Loaf command leaves and does not grant unclassified/operator commands, a bare Loaf namespace, or general filesystem access. Other harness adapters are not implied.",
		"",
		"See [orchestration skill](skills/orchestration/SKILL.md) for full details.",
		fencedEndMarker,
	}, "\n")
}

func canonicalInstallPath(path string) string {
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		if abs, absErr := filepath.Abs(realPath); absErr == nil {
			return abs
		}
		return realPath
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}
