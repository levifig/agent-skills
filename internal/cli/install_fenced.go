package cli

import (
	"crypto/sha256"
	"encoding/hex"
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
	"cursor":      "AGENTS.md",
	"codex":       "AGENTS.md",
	"opencode":    "AGENTS.md",
	"amp":         "AGENTS.md",
}

var fencedVersionField = regexp.MustCompile(`^v([\d.]+(?:-[-\w.]+)?)$`)

type fencedSectionRange struct {
	start           int
	end             int
	version         string // non-empty only for legacy headers; retained to detect legacy forms
	fingerprint     string
	malformedHeader bool
	bodyStart       int
}

type fencedInstallResult struct {
	Action  string
	Version string
	Error   string
}

// fencedSectionDisposition is the shared plan/apply decision for an existing
// managed section whose header already parsed successfully (not malformed).
type fencedSectionDisposition int

const (
	fencedDispositionSkip fencedSectionDisposition = iota
	fencedDispositionUpdate
	fencedDispositionTampered
)

// disposeFencedSection implements the normative header decision matrix from the
// version-agnostic-managed-block Change. Both installFencedSection (apply) and
// planFencedSection (dry-run) must consume this helper — Decision 8.
func disposeFencedSection(section fencedSectionRange, existingBody string, generatedFingerprint string) fencedSectionDisposition {
	actualSHA := sha256Hex(existingBody)
	if section.fingerprint != "" && section.fingerprint != actualSHA {
		return fencedDispositionTampered
	}
	// Skip requires new-form header (sha256= only, no version stamp) and a
	// fingerprint that matches both the actual body and the generated constant.
	if section.version == "" && section.fingerprint != "" && section.fingerprint == generatedFingerprint {
		return fencedDispositionSkip
	}
	return fencedDispositionUpdate
}

func installFencedSection(targetFile string, version string, upgrade bool) (fencedInstallResult, error) {
	canonicalTarget, err := canonicalFenceWritePath(targetFile)
	if err != nil {
		return fencedInstallResult{}, err
	}
	targetFile = canonicalTarget
	if version == "" {
		version = "0.0.0"
	}
	body, err := os.ReadFile(targetFile)
	fileExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fencedInstallResult{}, err
	}
	content := string(body)
	if err := validateFencedStructure(content); err != nil {
		return fencedInstallResult{}, err
	}
	section, hasSection := findFencedSectionRange(content)
	newContent := generateFencedContent()

	switch {
	case hasSection:
		if section.malformedHeader {
			return fencedInstallResult{}, fmt.Errorf("managed Loaf section in %s has a malformed fingerprint; refusing to overwrite", targetFile)
		}
		existingBody := content[section.bodyStart:section.end]
		switch disposeFencedSection(section, existingBody, fencedContentFingerprint(newContent)) {
		case fencedDispositionTampered:
			return fencedInstallResult{}, fmt.Errorf("managed Loaf section in %s was modified; refusing to overwrite", targetFile)
		case fencedDispositionSkip:
			return fencedInstallResult{Action: "skipped", Version: version}, nil
		}
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
		if err := writeFileAtomically(targetFile, []byte(updated), fencedWriteMode(targetFile, true)); err != nil {
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
		if err := writeFileAtomically(targetFile, []byte(updated), fencedWriteMode(targetFile, true)); err != nil {
			return fencedInstallResult{}, err
		}
		return fencedInstallResult{Action: "appended", Version: version}, nil
	default:
		if err := writeFileAtomically(targetFile, []byte(newContent+"\n"), 0o644); err != nil {
			return fencedInstallResult{}, err
		}
		return fencedInstallResult{Action: "created", Version: version}, nil
	}
}

func fencedWriteMode(path string, existed bool) os.FileMode {
	if existed {
		if info, err := os.Stat(path); err == nil {
			return info.Mode().Perm()
		}
	}
	return 0o644
}

func canonicalFenceWritePath(path string) (string, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	return filepath.EvalSymlinks(path)
}

func validateFencedStructure(content string) error {
	starts, ends := strings.Count(content, fencedStartMarker), strings.Count(content, fencedEndMarker)
	if starts == 0 && ends == 0 {
		return nil
	}
	if starts != 1 || ends != 1 {
		return fmt.Errorf("managed Loaf section has invalid fence structure; refusing to overwrite")
	}
	start, end := strings.Index(content, fencedStartMarker), strings.Index(content, fencedEndMarker)
	if start > end {
		return fmt.Errorf("managed Loaf section has invalid fence structure; refusing to overwrite")
	}
	return nil
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
	lineEnd := strings.IndexByte(content[start:], '\n')
	if lineEnd < 0 {
		lineEnd = len(content) - start
	}
	startLineEnd := strings.Index(content[start:start+lineEnd], "-->")
	if startLineEnd >= 0 && start+startLineEnd < start+endStart {
		version, sha, valid := parseFencedStartHeader(content[start : start+startLineEnd+3])
		if !valid {
			return fencedSectionRange{start: start, end: end, malformedHeader: true}, true
		}
		bodyStart := start + startLineEnd + 3
		if bodyStart < end && content[bodyStart] == '\n' {
			bodyStart++
		}
		return fencedSectionRange{start: start, end: end, version: version, fingerprint: sha, bodyStart: bodyStart}, true
	}
	return fencedSectionRange{start: start, end: end, malformedHeader: true}, true
}

func parseFencedStartHeader(line string) (string, string, bool) {
	if !strings.HasPrefix(line, fencedStartMarker) || !strings.HasSuffix(line, "-->") {
		return "", "", false
	}
	fields := strings.Fields(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, fencedStartMarker), "-->")))
	if len(fields) < 1 || len(fields) > 2 {
		return "", "", false
	}
	const shaPrefix = "sha256="
	parseSHA := func(field string) (string, bool) {
		if !strings.HasPrefix(field, shaPrefix) || len(field) != len(shaPrefix)+64 || !isCanonicalFenceSHA256(field[len(shaPrefix):]) {
			return "", false
		}
		return field[len(shaPrefix):], true
	}

	if len(fields) == 1 {
		// New form: sha256=<hex> alone.
		if sha, ok := parseSHA(fields[0]); ok {
			return "", sha, true
		}
		// Legacy: v<version> alone.
		match := fencedVersionField.FindStringSubmatch(fields[0])
		if len(match) != 2 {
			return "", "", false
		}
		return match[1], "", true
	}

	// Legacy: v<version> sha256=<hex>.
	match := fencedVersionField.FindStringSubmatch(fields[0])
	if len(match) != 2 {
		return "", "", false
	}
	sha, ok := parseSHA(fields[1])
	if !ok {
		return "", "", false
	}
	return match[1], sha, true
}

func isCanonicalFenceSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func generateFencedContent() string {
	body := strings.Join([]string{
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
		"See the Loaf `orchestration` skill for full details.",
		fencedEndMarker,
	}, "\n")
	return "<!-- loaf:managed:start sha256=" + sha256Hex(body) + " -->\n" + body
}

func fencedContentFingerprint(content string) string {
	section, ok := findFencedSectionRange(content)
	if !ok {
		return ""
	}
	return sha256Hex(content[section.bodyStart:section.end])
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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
