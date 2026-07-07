package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type releaseOptions struct {
	dryRun      bool
	help        bool
	bump        string
	base        string
	tagSet      bool
	tag         bool
	ghSet       bool
	gh          bool
	yes         bool
	preMerge    bool
	postMerge   bool
	versionFile []string
}

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

type releaseArtifactCommand struct {
	Label string
	Cwd   string
}

type releaseIncompleteTask struct {
	filename string
	status   string
}

type releaseVersionUpdate struct {
	path         string
	relativePath string
	oldVersion   string
	content      string
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

func parseReleaseArgs(args []string) (releaseOptions, error) {
	var options releaseOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h" || arg == "help":
			options.help = true
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "--bump":
			value, err := consumeFlagValue(args, &i, "--bump")
			if err != nil {
				return releaseOptions{}, err
			}
			options.bump = value
		case strings.HasPrefix(arg, "--bump="):
			options.bump = strings.TrimPrefix(arg, "--bump=")
			if options.bump == "" {
				return releaseOptions{}, fmt.Errorf("--bump requires a value")
			}
		case arg == "--base":
			value, err := consumeFlagValue(args, &i, "--base")
			if err != nil {
				return releaseOptions{}, err
			}
			options.base = value
		case strings.HasPrefix(arg, "--base="):
			options.base = strings.TrimPrefix(arg, "--base=")
			if options.base == "" {
				return releaseOptions{}, fmt.Errorf("--base requires a value")
			}
		case arg == "--no-tag":
			options.tagSet = true
			options.tag = false
		case arg == "--tag":
			options.tagSet = true
			options.tag = true
		case arg == "--no-gh":
			options.ghSet = true
			options.gh = false
		case arg == "--gh":
			options.ghSet = true
			options.gh = true
		case arg == "--version-file":
			value, err := consumeFlagValue(args, &i, "--version-file")
			if err != nil {
				return releaseOptions{}, err
			}
			options.versionFile = append(options.versionFile, normalizeReleasePath(value))
		case strings.HasPrefix(arg, "--version-file="):
			value := strings.TrimPrefix(arg, "--version-file=")
			if value == "" {
				return releaseOptions{}, fmt.Errorf("--version-file requires a value")
			}
			options.versionFile = append(options.versionFile, normalizeReleasePath(value))
		case arg == "--pre-merge":
			options.preMerge = true
		case arg == "--post-merge":
			options.postMerge = true
		case arg == "--yes" || arg == "-y":
			options.yes = true
		default:
			return releaseOptions{}, fmt.Errorf("unknown release option %q", arg)
		}
	}
	if options.bump != "" && !releaseValidBumps[options.bump] {
		return releaseOptions{}, fmt.Errorf("Invalid bump type %q. Must be one of: major, minor, patch, prerelease, release", options.bump)
	}
	if options.postMerge {
		var incompatible []string
		if options.bump != "" {
			incompatible = append(incompatible, "--bump")
		}
		if options.dryRun {
			incompatible = append(incompatible, "--dry-run")
		}
		if options.tagSet {
			if options.tag {
				incompatible = append(incompatible, "--tag")
			} else {
				incompatible = append(incompatible, "--no-tag")
			}
		}
		if options.ghSet {
			if options.gh {
				incompatible = append(incompatible, "--gh")
			} else {
				incompatible = append(incompatible, "--no-gh")
			}
		}
		if options.base != "" {
			incompatible = append(incompatible, "--base")
		}
		if len(options.versionFile) > 0 {
			incompatible = append(incompatible, "--version-file")
		}
		if options.yes {
			incompatible = append(incompatible, "--yes")
		}
		if options.preMerge {
			incompatible = append(incompatible, "--pre-merge")
		}
		if len(incompatible) > 0 {
			return releaseOptions{}, fmt.Errorf("--post-merge is incompatible with %s", strings.Join(incompatible, ", "))
		}
	}
	return options, nil
}

func runReleaseDryRun(root string, options releaseOptions, out io.Writer, errOut io.Writer) error {
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf release"))
	if !releaseIsGitRepo(root) {
		return fmt.Errorf("Not a git repository")
	}
	for _, declared := range options.versionFile {
		if !pathExistsNative(filepath.Join(root, declared)) {
			return fmt.Errorf("version file %s not found", declared)
		}
	}
	if options.preMerge {
		if options.tagSet && options.tag {
			fmt.Fprintf(errOut, "  %s --tag overrides --pre-merge default (no tag); proceeding with tag enabled\n", ansiYellow("warning:"))
		} else {
			options.tagSet = true
			options.tag = false
		}
		if options.ghSet && options.gh {
			fmt.Fprintf(errOut, "  %s --gh overrides --pre-merge default (no gh release); proceeding with GitHub release enabled\n", ansiYellow("warning:"))
		} else {
			options.ghSet = true
			options.gh = false
		}
		if options.base == "" {
			base, source, err := detectReleaseBase(root)
			if err != nil {
				return err
			}
			options.base = base
			fmt.Fprintf(out, "  %s %s %s\n", ansiCyan("Auto-detected base:"), ansiBold(base), ansiGray("(via "+source+")"))
		}
	}

	fmt.Fprintf(out, "  %s...\n\n", ansiCyan("Analyzing"))
	baseRef := releaseLastTag(root)
	if options.base != "" {
		resolved, err := validateReleaseBaseRef(root, options.base)
		if err != nil {
			return err
		}
		baseRef = resolved
	}
	commits := releaseCommitsSince(root, baseRef)
	if options.base != "" {
		if baseRef == options.base {
			fmt.Fprintf(out, "  Base ref: %s (via --base flag)\n", ansiBold(options.base))
		} else {
			fmt.Fprintf(out, "  Base ref: %s (resolved to %s via --base flag)\n", ansiBold(options.base), ansiBold(baseRef))
		}
	} else if baseRef != "" {
		fmt.Fprintf(out, "  Last tag: %s\n", ansiBold(baseRef))
	} else {
		fmt.Fprintf(out, "  Last tag: %s\n", ansiGray("(none)"))
	}
	if options.base != "" {
		fmt.Fprintf(out, "  Commits since base: %s\n\n", ansiBold(strconv.Itoa(len(commits))))
	} else {
		fmt.Fprintf(out, "  Commits since tag: %s\n\n", ansiBold(strconv.Itoa(len(commits))))
	}
	if len(commits) == 0 {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("No unreleased changes found."))
		return nil
	}
	for _, commit := range commits {
		if commit.Section == "" {
			fmt.Fprintf(out, "  %s  %s\n", ansiGray(fmt.Sprintf("%s (%s)", commit.Raw, commit.Hash)), ansiGray("[filtered]"))
		} else {
			fmt.Fprintf(out, "  %s\n", ansiGreen(fmt.Sprintf("%s (%s)", commit.Raw, commit.Hash)))
		}
	}
	if len(commits) > 0 {
		fmt.Fprintln(out)
	}

	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return err
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 && len(configOverrides) > 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil {
		return err
	}
	if len(versionFiles) == 0 {
		return fmt.Errorf("No version files found")
	}

	currentVersion := versionFiles[0].CurrentVersion
	commitBump := suggestReleaseBump(commits)
	bump := commitBump
	if options.bump != "" {
		bump = options.bump
	}
	newVersion := bumpReleaseVersion(currentVersion, bump)
	if newVersion == "" {
		return fmt.Errorf("Could not compute new version from %q", currentVersion)
	}
	if options.bump != "" {
		fmt.Fprintf(out, "  Bump type: %s (via --bump flag)\n\n", ansiBold(bump))
	}

	changelog := releaseChangelogSection(root, newVersion, time.Now().UTC().Format("2006-01-02"), commits)
	fmt.Fprintf(out, "  %s\n\n", ansiBold("Generated changelog:"))
	for _, line := range strings.Split(changelog, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s\n\n", ansiGray("(Set $EDITOR to edit before confirming)"))

	fmt.Fprintf(out, "  %s\n", ansiBold("Version files:"))
	for _, file := range versionFiles {
		fmt.Fprintf(out, "    • %s (%s → %s)\n", file.RelativePath, file.CurrentVersion, newVersion)
	}
	fmt.Fprintln(out)

	incompleteTasks := scanReleaseIncompleteTasks(root)
	if len(incompleteTasks) > 0 {
		fmt.Fprintf(out, "  %s %d\n", ansiBold("Incomplete tasks:"), len(incompleteTasks))
		for _, task := range incompleteTasks {
			fmt.Fprintf(out, "    %s %s (status: %s)\n", ansiYellow("⚠"), task.filename, task.status)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, "  Suggested bump: %s (%s)\n", ansiBold(bump), releaseBumpReason(bump))
	fmt.Fprintf(out, "  New version: %s\n\n", ansiBold(newVersion))

	skipTag, skipGh := normalizeReleaseSkipFlags(options)
	tagName := "v" + newVersion
	artifactCommands := releaseArtifactCommandsFor(root, versionFiles)
	fmt.Fprintf(out, "  %s\n", ansiBold("Actions:"))
	actionNum := 1
	fmt.Fprintf(out, "    %d. Update version in %d file(s)\n", actionNum, len(versionFiles))
	actionNum++
	fmt.Fprintf(out, "    %d. Update CHANGELOG.md\n", actionNum)
	actionNum++
	for _, command := range artifactCommands {
		fmt.Fprintf(out, "    %d. Run %s\n", actionNum, displayReleaseArtifactCommand(root, command))
		actionNum++
	}
	fmt.Fprintf(out, "    %d. Commit release artifacts\n", actionNum)
	actionNum++
	if skipTag {
		fmt.Fprintf(out, "    %s\n", ansiGray(fmt.Sprintf("%d. Create git tag %s (--no-tag — skipped)", actionNum, tagName)))
	} else {
		fmt.Fprintf(out, "    %d. Create git tag %s\n", actionNum, tagName)
	}
	actionNum++
	if skipGh {
		fmt.Fprintf(out, "    %s\n", ansiGray(fmt.Sprintf("%d. Create GitHub release draft (--no-gh — skipped)", actionNum)))
	} else if releaseGhAvailable() {
		fmt.Fprintf(out, "    %d. Create GitHub release draft (gh available)\n", actionNum)
	} else {
		fmt.Fprintf(out, "    %s\n", ansiGray(fmt.Sprintf("%d. Create GitHub release draft (gh not available — skipped)", actionNum)))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s No changes made.\n\n", ansiCyan("--dry-run:"))
	return nil
}

func runReleaseApply(root string, options releaseOptions, in io.Reader, out io.Writer, errOut io.Writer) error {
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf release"))
	if !releaseIsGitRepo(root) {
		return fmt.Errorf("Not a git repository")
	}
	for _, declared := range options.versionFile {
		if !pathExistsNative(filepath.Join(root, declared)) {
			return fmt.Errorf("version file %s not found", declared)
		}
	}
	if options.preMerge {
		if options.tagSet && options.tag {
			fmt.Fprintf(errOut, "  %s --tag overrides --pre-merge default (no tag); proceeding with tag enabled\n", ansiYellow("warning:"))
		} else {
			options.tagSet = true
			options.tag = false
		}
		if options.ghSet && options.gh {
			fmt.Fprintf(errOut, "  %s --gh overrides --pre-merge default (no gh release); proceeding with GitHub release enabled\n", ansiYellow("warning:"))
		} else {
			options.ghSet = true
			options.gh = false
		}
		if options.base == "" {
			base, source, err := detectReleaseBase(root)
			if err != nil {
				return err
			}
			options.base = base
			fmt.Fprintf(out, "  %s %s %s\n", ansiCyan("Auto-detected base:"), ansiBold(base), ansiGray("(via "+source+")"))
		}
	}

	fmt.Fprintf(out, "  %s...\n\n", ansiCyan("Analyzing"))
	baseRef := releaseLastTag(root)
	if options.base != "" {
		resolved, err := validateReleaseBaseRef(root, options.base)
		if err != nil {
			return err
		}
		baseRef = resolved
	}
	commits := releaseCommitsSince(root, baseRef)
	if options.base != "" {
		if baseRef == options.base {
			fmt.Fprintf(out, "  Base ref: %s (via --base flag)\n", ansiBold(options.base))
		} else {
			fmt.Fprintf(out, "  Base ref: %s (resolved to %s via --base flag)\n", ansiBold(options.base), ansiBold(baseRef))
		}
	} else if baseRef != "" {
		fmt.Fprintf(out, "  Last tag: %s\n", ansiBold(baseRef))
	} else {
		fmt.Fprintf(out, "  Last tag: %s\n", ansiGray("(none)"))
	}
	if options.base != "" {
		fmt.Fprintf(out, "  Commits since base: %s\n\n", ansiBold(strconv.Itoa(len(commits))))
	} else {
		fmt.Fprintf(out, "  Commits since tag: %s\n\n", ansiBold(strconv.Itoa(len(commits))))
	}
	if len(commits) == 0 {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("No unreleased changes found."))
		return nil
	}
	for _, commit := range commits {
		if commit.Section == "" {
			fmt.Fprintf(out, "  %s  %s\n", ansiGray(fmt.Sprintf("%s (%s)", commit.Raw, commit.Hash)), ansiGray("[filtered]"))
		} else {
			fmt.Fprintf(out, "  %s\n", ansiGreen(fmt.Sprintf("%s (%s)", commit.Raw, commit.Hash)))
		}
	}
	fmt.Fprintln(out)

	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return err
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 && len(configOverrides) > 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil {
		return err
	}
	if len(versionFiles) == 0 {
		return fmt.Errorf("No version files found")
	}

	currentVersion := versionFiles[0].CurrentVersion
	bump := suggestReleaseBump(commits)
	if options.bump != "" {
		bump = options.bump
	}
	newVersion := bumpReleaseVersion(currentVersion, bump)
	if newVersion == "" {
		return fmt.Errorf("Could not compute new version from %q", currentVersion)
	}
	if options.bump != "" {
		fmt.Fprintf(out, "  Bump type: %s (via --bump flag)\n\n", ansiBold(bump))
	}

	changelog := releaseChangelogSection(root, newVersion, time.Now().UTC().Format("2006-01-02"), commits)
	fmt.Fprintf(out, "  %s\n\n", ansiBold("Generated changelog:"))
	for _, line := range strings.Split(changelog, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "  %s\n", ansiBold("Version files:"))
	for _, file := range versionFiles {
		fmt.Fprintf(out, "    • %s (%s → %s)\n", file.RelativePath, file.CurrentVersion, newVersion)
	}
	fmt.Fprintln(out)

	skipTag, skipGh := normalizeReleaseSkipFlags(options)
	tagName := "v" + newVersion
	artifactCommands := releaseArtifactCommandsFor(root, versionFiles)
	fmt.Fprintf(out, "  Suggested bump: %s (%s)\n", ansiBold(bump), releaseBumpReason(bump))
	fmt.Fprintf(out, "  New version: %s\n\n", ansiBold(newVersion))
	if !options.yes {
		confirmed, err := confirmRelease(in, out, tagName)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintf(out, "\n  %s\n\n", ansiGray("Release cancelled."))
			return nil
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  %s\n", ansiBold("Executing:"))

	updates, err := prepareReleaseVersionUpdates(versionFiles, newVersion)
	if err != nil {
		return fmt.Errorf("Failed to update version files: %w", err)
	}
	for _, update := range updates {
		if err := os.WriteFile(update.path, []byte(update.content), 0o644); err != nil {
			return fmt.Errorf("Failed to update %s: %w", update.relativePath, err)
		}
		fmt.Fprintf(out, "    %s Updated %s (%s → %s)\n", ansiGreen("✓"), update.relativePath, update.oldVersion, newVersion)
	}

	if err := writeReleaseChangelog(root, changelog); err != nil {
		return fmt.Errorf("Failed to update CHANGELOG.md: %w", err)
	}
	fmt.Fprintf(out, "    %s Updated CHANGELOG.md\n", ansiGreen("✓"))

	for _, command := range artifactCommands {
		if err := runReleaseArtifactCommand(root, command, out, errOut); err != nil {
			return fmt.Errorf("Release artifact command failed: %w", err)
		}
	}
	if paths := unignoredReleaseVirtualEnvStatusPaths(root); len(paths) > 0 {
		return fmt.Errorf("Refusing to commit release artifacts: unignored virtual environment path detected: %s", strings.Join(paths, ", "))
	}

	if err := releaseCommandRun(root, "git", "add", "-A"); err != nil {
		return fmt.Errorf("Failed to stage release artifacts: %w", err)
	}
	if err := releaseCommandRun(root, "git", "commit", "-m", "chore: release "+tagName); err != nil {
		return fmt.Errorf("Failed to commit release: %w", err)
	}
	fmt.Fprintf(out, "    %s Committed release artifacts\n", ansiGreen("✓"))

	if skipTag {
		fmt.Fprintf(out, "    %s Git tag skipped (--no-tag)\n", ansiGray("-"))
	} else {
		if err := releaseCommandRun(root, "git", "tag", "-s", tagName, "-m", "Release "+newVersion); err != nil {
			return fmt.Errorf("Failed to create tag: %w", err)
		}
		fmt.Fprintf(out, "    %s Created tag %s\n", ansiGreen("✓"), tagName)
	}

	if skipGh {
		fmt.Fprintf(out, "    %s GitHub release skipped (--no-gh)\n", ansiGray("-"))
	} else if releaseGhAvailable() {
		if err := verifyConfiguredGitHubAccount(root, out); err != nil {
			return fmt.Errorf("Refusing to create GitHub release with the wrong account: %w", err)
		}
		if err := releaseCommandRun(root, "gh", "release", "create", tagName, "--draft", "--title", "v"+newVersion, "--notes", changelog); err != nil {
			return fmt.Errorf("Failed to create GitHub release: %w", err)
		}
		fmt.Fprintf(out, "    %s Created GitHub release draft\n", ansiGreen("✓"))
	} else {
		fmt.Fprintf(out, "    %s GitHub release skipped (gh not available)\n", ansiGray("-"))
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s Release %s complete\n\n", ansiGreen("✓"), ansiBold(tagName))
	return nil
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

func releaseCommitsSince(root string, base string) []releaseCommit {
	format := "%h%x00%s%x00%B%x00"
	args := []string{"log", "--format=" + format}
	if base != "" {
		args = []string{"log", base + "..HEAD", "--format=" + format}
	}
	output := releaseCommandOutput(root, "git", args...)
	if strings.TrimSpace(output) == "" {
		return nil
	}
	var commits []releaseCommit
	for _, chunk := range strings.Split(output, "\x00\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		parts := strings.Split(chunk, "\x00")
		if len(parts) < 2 {
			continue
		}
		hash := strings.TrimSpace(parts[0])
		subject := strings.TrimSpace(parts[1])
		body := ""
		if len(parts) > 2 {
			body = strings.TrimSpace(parts[2])
		}
		if hash == "" || subject == "" {
			continue
		}
		commits = append(commits, parseReleaseCommit(hash, subject, body))
	}
	return commits
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

func suggestReleaseBump(commits []releaseCommit) string {
	for _, commit := range commits {
		if commit.Breaking {
			return "major"
		}
	}
	for _, commit := range commits {
		if commit.Section == "Added" {
			return "minor"
		}
	}
	return "patch"
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
	body, err := os.ReadFile(path)
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

func releaseConfigVersionFiles(root string) ([]string, error) {
	body, err := os.ReadFile(filepath.Join(root, ".agents", "loaf.json"))
	if err != nil {
		return nil, nil
	}
	var config struct {
		Release struct {
			VersionFiles []string `json:"versionFiles"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, nil
	}
	var values []string
	for _, value := range config.Release.VersionFiles {
		if normalized := normalizeReleasePath(value); normalized != "" {
			values = append(values, normalized)
		}
	}
	return values, nil
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

func releaseChangelogSection(root string, version string, date string, commits []releaseCommit) string {
	body, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err == nil {
		if curated := extractReleaseUnreleasedBody(string(body)); curated != "" {
			return fmt.Sprintf("## [%s] - %s\n\n%s", version, date, curated)
		}
	}
	grouped := map[string][]releaseCommit{}
	for _, commit := range commits {
		if commit.Section == "" {
			continue
		}
		grouped[commit.Section] = append(grouped[commit.Section], commit)
	}
	lines := []string{fmt.Sprintf("## [%s] - %s", version, date)}
	for _, section := range []string{"Breaking Changes", "Added", "Changed", "Fixed", "Other"} {
		commits := grouped[section]
		if len(commits) == 0 {
			continue
		}
		lines = append(lines, "", "### "+section)
		for _, commit := range commits {
			lines = append(lines, fmt.Sprintf("- %s (%s)", capitalizeReleaseMessage(commit.Message), commit.Hash))
		}
	}
	return strings.Join(lines, "\n")
}

func extractReleaseUnreleasedBody(content string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if releaseUnreleasedHeadingRE.MatchString(strings.TrimSpace(line)) {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## [") {
			end = i
			break
		}
	}
	var filtered []string
	for _, line := range lines[start:end] {
		if releaseUnreleasedStubRE.MatchString(line) {
			continue
		}
		filtered = append(filtered, line)
	}
	for len(filtered) > 0 && strings.TrimSpace(filtered[0]) == "" {
		filtered = filtered[1:]
	}
	for len(filtered) > 0 && strings.TrimSpace(filtered[len(filtered)-1]) == "" {
		filtered = filtered[:len(filtered)-1]
	}
	for _, line := range filtered {
		if strings.TrimSpace(line) != "" {
			return strings.Join(filtered, "\n")
		}
	}
	return ""
}

func releaseArtifactCommandsFor(root string, versionFiles []releaseVersionFile) []releaseArtifactCommand {
	var commands []releaseArtifactCommand
	seen := map[string]bool{}
	add := func(label string, cwd string) {
		key := cwd + "\x00" + label
		if seen[key] {
			return
		}
		seen[key] = true
		commands = append(commands, releaseArtifactCommand{Label: label, Cwd: cwd})
	}
	for _, file := range versionFiles {
		dir := filepath.Dir(file.Path)
		if filepath.Base(file.RelativePath) == "pyproject.toml" && pathExistsNative(filepath.Join(dir, "uv.lock")) {
			add("uv sync", dir)
		}
	}
	for _, file := range versionFiles {
		dir := filepath.Dir(file.Path)
		if filepath.Base(file.RelativePath) == "package.json" && releasePackageHasBuildScript(file.Path) {
			add("npm run build", dir)
		}
	}
	if releasePackageHasBuildScript(filepath.Join(root, "package.json")) {
		add("npm run build", root)
	}
	if len(commands) == 0 {
		add("loaf build", root)
	}
	return commands
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
		body, err := os.ReadFile(filepath.Join(tasksDir, entry.Name()))
		if err != nil {
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

func confirmRelease(in io.Reader, out io.Writer, tagName string) (bool, error) {
	if !readerIsTerminal(in) {
		if in == nil {
			return false, nil
		}
		if _, ok := in.(*os.File); ok {
			return false, nil
		}
	}
	reader := bufio.NewReader(in)
	fmt.Fprintf(out, "  Proceed with release %s? [y/N] ", ansiBold(tagName))
	answer, err := reader.ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y"), nil
}

func prepareReleaseVersionUpdates(files []releaseVersionFile, newVersion string) ([]releaseVersionUpdate, error) {
	var updates []releaseVersionUpdate
	for _, file := range files {
		body, err := os.ReadFile(file.Path)
		if err != nil {
			return nil, err
		}
		var updated string
		switch file.Format {
		case "json":
			re := regexp.MustCompile(`"version"(\s*:\s*)"` + regexp.QuoteMeta(file.CurrentVersion) + `"`)
			updated = re.ReplaceAllString(string(body), `"version"$1"`+newVersion+`"`)
		case "toml-regex":
			section := releaseTomlSectionForPath(file.RelativePath)
			if section == "" {
				continue
			}
			updated = replaceReleaseTomlVersion(string(body), section, newVersion)
		default:
			continue
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
	body, err := os.ReadFile(path)
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

func runReleaseArtifactCommand(root string, command releaseArtifactCommand, out io.Writer, errOut io.Writer) error {
	executable, args, err := releaseArtifactInvocation(root, command)
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = command.Cwd
	cmd.Stdout = out
	cmd.Stderr = errOut
	if err := cmd.Run(); err != nil {
		return err
	}
	suffix := ""
	if rel, err := filepath.Rel(root, command.Cwd); err == nil && rel != "." {
		suffix = " in " + filepath.ToSlash(rel)
	}
	fmt.Fprintf(out, "    %s Ran %s%s\n", ansiGreen("✓"), command.Label, suffix)
	return nil
}

func releaseArtifactInvocation(root string, command releaseArtifactCommand) (string, []string, error) {
	switch command.Label {
	case "uv sync":
		path, err := exec.LookPath("uv")
		return path, []string{"sync"}, err
	case "npm run build":
		path, err := exec.LookPath("npm")
		return path, []string{"run", "build"}, err
	case "loaf build":
		executable, err := os.Executable()
		if err != nil {
			return "", nil, err
		}
		return executable, []string{"build"}, nil
	default:
		return "", nil, fmt.Errorf("unknown release artifact command %q in %s", command.Label, root)
	}
}

func unignoredReleaseVirtualEnvStatusPaths(root string) []string {
	output := releaseCommandOutput(root, "git", "status", "--porcelain", "--untracked-files=all", "-z")
	if output == "" {
		return nil
	}
	paths := map[string]bool{}
	for _, entry := range strings.Split(output, "\x00") {
		if entry == "" {
			continue
		}
		path := entry
		if len(entry) > 3 && entry[2] == ' ' {
			path = entry[3:]
		}
		normalized := filepath.ToSlash(path)
		if strings.Contains("/"+normalized+"/", "/.venv/") {
			paths[normalized] = true
		}
	}
	var values []string
	for path := range paths {
		values = append(values, path)
	}
	sort.Strings(values)
	return values
}

func releasePackageHasBuildScript(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var parsed struct {
		Scripts map[string]string `json:"scripts"`
	}
	return json.Unmarshal(body, &parsed) == nil && parsed.Scripts["build"] != ""
}

func displayReleaseArtifactCommand(root string, command releaseArtifactCommand) string {
	rel, err := filepath.Rel(root, command.Cwd)
	if err != nil || rel == "." {
		return command.Label
	}
	return fmt.Sprintf("%s (%s)", command.Label, filepath.ToSlash(rel))
}

func normalizeReleaseSkipFlags(options releaseOptions) (bool, bool) {
	skipTag := options.tagSet && !options.tag
	skipGh := (options.ghSet && !options.gh) || skipTag
	return skipTag, skipGh
}

func detectReleaseBase(root string) (string, string, error) {
	if current := releaseCommandOutput(root, "git", "symbolic-ref", "--short", "HEAD"); current == "" {
		return "", "", fmt.Errorf("--pre-merge requires a named branch (detached HEAD detected). Pass --base <ref> explicitly")
	}
	if config := releaseCommandOutput(root, "git", "config", "--get", "loaf.release.base"); config != "" {
		return config, "config", nil
	}
	if defaultBranch := releaseCommandOutput(root, "gh", "repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name"); defaultBranch != "" {
		return defaultBranch, "default", nil
	}
	if symRef := releaseCommandOutput(root, "git", "symbolic-ref", "refs/remotes/origin/HEAD"); strings.HasPrefix(symRef, "refs/remotes/origin/") {
		return strings.TrimPrefix(symRef, "refs/remotes/origin/"), "default", nil
	}
	return "", "", fmt.Errorf("Could not auto-detect base branch. Pass --base <ref> explicitly, or set git config loaf.release.base <ref>")
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

func releaseBumpReason(bump string) string {
	switch bump {
	case "major":
		return "breaking changes detected"
	case "minor":
		return "new features detected"
	case "patch":
		return "bug fixes only"
	case "prerelease":
		return "development milestone"
	case "release":
		return "stable release"
	default:
		return "selected bump"
	}
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
