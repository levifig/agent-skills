package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type releaseVersionFile struct {
	Path           string
	RelativePath   string
	Format         string
	CurrentVersion string
}

type releaseCommit struct {
	Hash     string
	Type     string
	Message  string
	Breaking bool
	Section  string
	Raw      string
}

type releaseVersionUpdate struct {
	path         string
	relativePath string
	oldVersion   string
	content      string
}

type releaseIncompleteTask struct {
	filename string
	status   string
}

var releaseConventionalCommitRE = regexp.MustCompile(`^(\w+)(\(.+?\))?(!)?:\s*(.+)$`)
var releaseBreakingBodyRE = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE:`)
var releaseUnreleasedHeadingRE = regexp.MustCompile(`(?i)^## \[unreleased\]`)
var releaseUnreleasedStubRE = regexp.MustCompile(`^[-*]\s+_No unreleased changes.*_\.?\s*$`)

var releaseValidBumps = map[string]bool{
	"major":      true,
	"minor":      true,
	"patch":      true,
	"prerelease": true,
	"release":    true,
}

func releaseIsGitRepo(root string) bool {
	return releaseCommandOutput(root, "git", "rev-parse", "--is-inside-work-tree") != ""
}

func releaseLastTag(root string) string {
	return releaseCommandOutput(root, "git", "describe", "--tags", "--abbrev=0")
}

func validateReleaseBaseRef(root string, ref string) (string, error) {
	candidates := []string{ref}
	if !strings.Contains(ref, "/") {
		candidates = append(candidates, "origin/"+ref)
	}
	for _, candidate := range candidates {
		if releaseCommandOK(root, "git", "rev-parse", "--verify", candidate+"^{commit}") {
			return candidate, nil
		}
	}
	var quoted []string
	for _, candidate := range candidates {
		quoted = append(quoted, fmt.Sprintf("%q", candidate))
	}
	return "", fmt.Errorf("Base ref %q does not exist or is not reachable. Tried %s. If this is a remote branch, run: git fetch origin %s", ref, strings.Join(quoted, " and "), ref)
}

func parseReleaseCommit(hash string, subject string, body string) releaseCommit {
	breakingFromBody := releaseBreakingBodyRE.MatchString(body)
	match := releaseConventionalCommitRE.FindStringSubmatch(subject)
	if match == nil {
		section := "Other"
		if breakingFromBody {
			section = "Breaking Changes"
		}
		return releaseCommit{Hash: hash, Message: subject, Breaking: breakingFromBody, Section: section, Raw: subject}
	}
	commitType := match[1]
	breaking := match[3] != "" || breakingFromBody
	section := releaseSectionForType(commitType, breaking)
	return releaseCommit{Hash: hash, Type: commitType, Message: match[4], Breaking: breaking, Section: section, Raw: subject}
}

func releaseSectionForType(commitType string, breaking bool) string {
	if breaking {
		return "Breaking Changes"
	}
	switch commitType {
	case "feat":
		return "Added"
	case "fix":
		return "Fixed"
	case "refactor", "perf":
		return "Changed"
	case "docs", "chore", "ci", "test", "build", "style":
		return ""
	default:
		return "Other"
	}
}

func detectReleaseVersionFiles(root string, overrides []string) ([]releaseVersionFile, error) {
	if len(overrides) > 0 {
		files := make([]releaseVersionFile, 0, len(overrides))
		for _, override := range overrides {
			file, err := loadReleaseVersionFile(root, override, true)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
		return files, nil
	}
	var ecosystem []releaseVersionFile
	var loafFile *releaseVersionFile
	for _, candidate := range []string{"package.json", "pyproject.toml", "Cargo.toml", ".agents/loaf.json", ".claude-plugin/marketplace.json"} {
		file, err := loadReleaseVersionFile(root, candidate, false)
		if err != nil {
			continue
		}
		if candidate == ".agents/loaf.json" {
			copy := file
			loafFile = &copy
		} else {
			ecosystem = append(ecosystem, file)
		}
	}
	if len(ecosystem) > 0 {
		return ecosystem, nil
	}
	if loafFile != nil {
		return []releaseVersionFile{*loafFile}, nil
	}
	return nil, nil
}

func loadReleaseVersionFile(root string, relativePath string, strict bool) (releaseVersionFile, error) {
	normalized := normalizeReleasePath(relativePath)
	path := filepath.Join(root, filepath.FromSlash(normalized))
	body, err := readRegularFile(path, projectFileReadLimit)
	if err != nil {
		if strict {
			return releaseVersionFile{}, fmt.Errorf("version file %s not found", normalized)
		}
		return releaseVersionFile{}, err
	}
	version, format, err := parseReleaseVersion(normalized, body)
	if err != nil {
		if strict {
			return releaseVersionFile{}, fmt.Errorf("version file %s: %v", normalized, err)
		}
		return releaseVersionFile{}, err
	}
	return releaseVersionFile{Path: path, RelativePath: normalized, Format: format, CurrentVersion: version}, nil
}

func parseReleaseVersion(relativePath string, body []byte) (string, string, error) {
	base := filepath.Base(filepath.FromSlash(relativePath))
	switch {
	case base == "package.json" || base == "loaf.json" || strings.HasSuffix(base, ".json"):
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return "", "", fmt.Errorf("could not parse version")
		}
		var version any
		if base == "marketplace.json" {
			if metadata, ok := raw["metadata"].(map[string]any); ok {
				version = metadata["version"]
			}
		} else {
			version = raw["version"]
		}
		if value, ok := version.(string); ok && value != "" {
			return value, "json", nil
		}
	case base == "pyproject.toml" || base == "Cargo.toml" || strings.HasSuffix(base, ".toml"):
		section := "project"
		if base == "Cargo.toml" {
			section = "package"
		}
		if version := readReleaseTomlVersion(string(body), section); version != "" {
			return version, "toml-regex", nil
		}
	default:
		return "", "", fmt.Errorf("unsupported file type (expected .json or .toml)")
	}
	return "", "", fmt.Errorf("could not parse version")
}

func readReleaseTomlVersion(content string, section string) string {
	lines := strings.Split(content, "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "["+section+"]" {
			inSection = true
			continue
		}
		if inSection {
			if strings.HasPrefix(trimmed, "[") {
				return ""
			}
			if strings.HasPrefix(trimmed, "version") {
				parts := strings.SplitN(trimmed, "=", 2)
				if len(parts) == 2 {
					return strings.Trim(strings.TrimSpace(parts[1]), `"`)
				}
			}
		}
	}
	return ""
}

func bumpReleaseVersion(current string, bump string) string {
	version, ok := parseReleaseSemver(current)
	if !ok {
		return ""
	}
	switch bump {
	case "major":
		return fmt.Sprintf("%d.0.0", version.major+1)
	case "minor":
		return fmt.Sprintf("%d.%d.0", version.major, version.minor+1)
	case "patch":
		return fmt.Sprintf("%d.%d.%d", version.major, version.minor, version.patch+1)
	case "prerelease":
		if version.prerelease == "" {
			return ""
		}
		label, suffix, found := strings.Cut(version.prerelease, ".")
		if !found {
			return fmt.Sprintf("%d.%d.%d-%s.1", version.major, version.minor, version.patch, version.prerelease)
		}
		lastDot := strings.LastIndex(version.prerelease, ".")
		label = version.prerelease[:lastDot]
		suffix = version.prerelease[lastDot+1:]
		number, err := strconv.Atoi(suffix)
		if err != nil || number < 0 {
			return fmt.Sprintf("%d.%d.%d-%s.1", version.major, version.minor, version.patch, version.prerelease)
		}
		return fmt.Sprintf("%d.%d.%d-%s.%d", version.major, version.minor, version.patch, label, number+1)
	case "release":
		if version.prerelease == "" {
			return ""
		}
		return fmt.Sprintf("%d.%d.%d", version.major, version.minor, version.patch)
	default:
		return ""
	}
}

type releaseSemver struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

// releaseVersionIsPrerelease reports whether a version carries a semver
// prerelease identifier. GitHub keeps a prerelease out of "Latest release" and
// labels it as such, which is what an alpha, beta, or release candidate should
// get; without the flag every alpha publishes as the project's latest stable.
func releaseVersionIsPrerelease(version string) bool {
	parsed, ok := parseReleaseSemver(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	return ok && parsed.prerelease != ""
}

func parseReleaseSemver(value string) (releaseSemver, bool) {
	core := value
	prerelease := ""
	if before, after, found := strings.Cut(value, "-"); found {
		core = before
		prerelease = after
		if prerelease == "" {
			return releaseSemver{}, false
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return releaseSemver{}, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || major < 0 || minor < 0 || patch < 0 {
		return releaseSemver{}, false
	}
	return releaseSemver{major: major, minor: minor, patch: patch, prerelease: prerelease}, true
}

func scanReleaseIncompleteTasks(root string) []releaseIncompleteTask {
	tasksDir := filepath.Join(root, ".agents", "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil
	}
	var incomplete []releaseIncompleteTask
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		body, err := readRegularFile(filepath.Join(tasksDir, entry.Name()), projectFileReadLimit)
		if err != nil {
			// Enumerated discovered path: skip non-regular or unreadable
			// entries rather than hanging the release dry-run on one of them.
			continue
		}
		lines := strings.Split(string(body), "\n")
		if len(lines) > 20 {
			lines = lines[:20]
		}
		for _, line := range lines {
			status, ok := strings.CutPrefix(strings.TrimSpace(line), "status:")
			if !ok {
				continue
			}
			value := strings.TrimSpace(status)
			if value != "complete" && value != "archived" {
				incomplete = append(incomplete, releaseIncompleteTask{filename: entry.Name(), status: value})
			}
			break
		}
	}
	return incomplete
}

func releaseGitShowPath(root, rev, relPath string) ([]byte, error) {
	spec := rev + ":" + filepath.ToSlash(relPath)
	cmd := exec.Command("git", "show", spec)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func releaseRenderVersionContent(relPath string, headBody []byte, currentVersion, format, candidate string) (string, error) {
	switch format {
	case "json":
		re := regexp.MustCompile(`"version"(\s*:\s*)"` + regexp.QuoteMeta(currentVersion) + `"`)
		if !re.Match(headBody) {
			return "", fmt.Errorf("version file %s does not contain version %s", relPath, currentVersion)
		}
		return re.ReplaceAllString(string(headBody), `"version"$1"`+candidate+`"`), nil
	case "toml-regex":
		section := releaseTomlSectionForPath(relPath)
		if section == "" {
			return "", fmt.Errorf("version file %s: no toml section", relPath)
		}
		return replaceReleaseTomlVersion(string(headBody), section, candidate), nil
	default:
		return "", fmt.Errorf("version file %s: unsupported format %s", relPath, format)
	}
}

func prepareReleaseVersionUpdates(root string, files []releaseVersionFile, newVersion string) ([]releaseVersionUpdate, error) {
	var updates []releaseVersionUpdate
	for _, file := range files {
		if file.Format != "json" && file.Format != "toml-regex" {
			continue
		}
		if file.Format == "toml-regex" && releaseTomlSectionForPath(file.RelativePath) == "" {
			continue
		}
		// Prefer HEAD content so an uncommitted candidate bump (resume after
		// evidence refusal) still rewrites from the baseline version string.
		body, err := releaseGitShowPath(root, "HEAD", file.RelativePath)
		if err != nil {
			body, err = readRegularFile(file.Path, projectFileReadLimit)
			if err != nil {
				return nil, err
			}
		}
		updated, err := releaseRenderVersionContent(file.RelativePath, body, file.CurrentVersion, file.Format, newVersion)
		if err != nil {
			return nil, err
		}
		updates = append(updates, releaseVersionUpdate{
			path:         file.Path,
			relativePath: file.RelativePath,
			oldVersion:   file.CurrentVersion,
			content:      updated,
		})
	}
	return updates, nil
}

// releaseNonEvidenceChangePaths filters docs/changes dirt down to paths that
// are not capability-evidence receipts/sources (which re-record on resume).
func releaseTomlSectionForPath(relativePath string) string {
	base := filepath.Base(filepath.FromSlash(relativePath))
	switch base {
	case "pyproject.toml":
		return "project"
	case "Cargo.toml":
		return "package"
	default:
		if strings.HasSuffix(base, ".toml") {
			return "project"
		}
		return ""
	}
}

func replaceReleaseTomlVersion(content string, section string, newVersion string) string {
	lines := strings.Split(content, "\n")
	inSection := false
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "["+section+"]" {
			inSection = true
			continue
		}
		if !inSection || replaced {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inSection = false
			continue
		}
		if regexp.MustCompile(`^\s*version\s*=`).MatchString(line) {
			lines[i] = regexp.MustCompile(`^(\s*version\s*=\s*)"[^"]+"`).ReplaceAllString(line, `$1"`+newVersion+`"`)
			replaced = true
		}
	}
	return strings.Join(lines, "\n")
}

func writeReleaseChangelog(root string, releaseSection string) error {
	path := filepath.Join(root, "CHANGELOG.md")
	body, err := readRegularFile(path, projectFileReadLimit)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(createReleaseChangelog(releaseSection)), 0o644)
		}
		return err
	}
	updated := insertReleaseChangelog(string(body), releaseSection)
	if updated == "" {
		updated = strings.TrimRight(string(body), "\n") + "\n\n" + releaseSection + "\n"
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func insertReleaseChangelog(existing string, releaseSection string) string {
	lines := strings.Split(existing, "\n")
	unreleased := -1
	for i, line := range lines {
		if releaseUnreleasedHeadingRE.MatchString(strings.TrimSpace(line)) {
			unreleased = i
			break
		}
	}
	if unreleased == -1 {
		return ""
	}
	nextRelease := len(lines)
	for i := unreleased + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## [") {
			nextRelease = i
			break
		}
	}
	result := append([]string{}, lines[:unreleased+1]...)
	result = append(result, "", "- _No unreleased changes yet._", "", releaseSection, "")
	result = append(result, lines[nextRelease:]...)
	return strings.Join(result, "\n")
}

func createReleaseChangelog(releaseSection string) string {
	return strings.Join([]string{
		"# Changelog",
		"",
		"This project follows [Common Changelog](https://common-changelog.org/) and",
		"[Semantic Versioning](https://semver.org/spec/v2.0.0.html). `## [Unreleased]`",
		"is a workflow staging section for curated entries before release.",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
		releaseSection,
		"",
	}, "\n")
}

func releaseGhAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

func releaseCommandOK(root string, name string, args ...string) bool {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	return cmd.Run() == nil
}

func releaseCommandRun(root string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}
	return nil
}

func releaseCommandOutput(root string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func capitalizeReleaseMessage(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func normalizeReleasePath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}
