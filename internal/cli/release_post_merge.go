package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type releasePostMergeCommandResult struct {
	stdout   string
	exitCode int
	notFound bool
}

type releasePostMergeCommandRunner func(root string, name string, args ...string) releasePostMergeCommandResult

type releasePostMergeResult struct {
	ok            bool
	guardrail     int
	message       string
	version       string
	base          string
	featureBranch string
	changelogBody string
	versionFiles  []releaseVersionFile
}

type releasePostMergeActionResult struct {
	tagged        bool
	pushed        bool
	released      bool
	pulled        bool
	deletedLocal  *bool
	deletedRemote *bool
}

var releasePRSuffixRE = regexp.MustCompile(`\(#(\d+)\)\s*$`)

func runReleasePostMerge(root string, out io.Writer, errOut io.Writer) error {
	return runReleasePostMergeWithRunner(root, out, errOut, defaultReleasePostMergeCommandRunner)
}

func runReleasePostMergeWithRunner(root string, out io.Writer, errOut io.Writer, runner releasePostMergeCommandRunner) error {
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf release"))
	fmt.Fprintf(out, "  %s...\n\n", ansiCyan("Verifying post-merge state"))

	result := checkReleasePostMergeGuardrails(root, runner)
	if !result.ok {
		return fmt.Errorf("guardrail %d failed: %s", result.guardrail, result.message)
	}

	fmt.Fprintf(out, "  %s All 8 guardrails passed for %s on %s\n", ansiGreen("✓"), ansiBold("v"+result.version), ansiBold(result.base))
	if result.featureBranch != "" {
		fmt.Fprintf(out, "  %s %s\n", ansiGray("feature branch:"), result.featureBranch)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s\n", ansiBold("Executing:"))

	if _, err := executeReleasePostMergeActions(root, result, runner, out, errOut); err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s Release v%s finalized\n\n", ansiGreen("✓"), result.version)
	return nil
}

func checkReleasePostMergeGuardrails(root string, runner releasePostMergeCommandRunner) releasePostMergeResult {
	if dirty := checkReleasePostMergeCleanWorktree(root, runner); dirty != "" {
		return releasePostMergeAbort(1, dirty)
	}

	currentResult := runner(root, "git", "symbolic-ref", "--short", "HEAD")
	if currentResult.exitCode != 0 || strings.TrimSpace(currentResult.stdout) == "" {
		return releasePostMergeAbort(2, "detached HEAD — checkout the base branch and rerun")
	}
	current := strings.TrimSpace(currentResult.stdout)

	base, err := detectReleasePostMergeBase(root, runner)
	if err != nil {
		return releasePostMergeAbort(2, err.Error())
	}
	if branchAbort := checkReleasePostMergeOnBase(root, runner, base, current); branchAbort != "" {
		return releasePostMergeAbort(2, branchAbort)
	}

	subjectResult := runner(root, "git", "log", "-1", "--pretty=%s")
	if subjectResult.exitCode != 0 {
		return releasePostMergeAbort(3, "could not read HEAD subject")
	}
	prNumber := ""
	if match := releasePRSuffixRE.FindStringSubmatch(strings.TrimSpace(subjectResult.stdout)); match != nil {
		prNumber = match[1]
	}

	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return releasePostMergeAbort(4, err.Error())
	}
	versionFiles, err := detectReleaseVersionFiles(root, configOverrides)
	if err != nil {
		return releasePostMergeAbort(4, err.Error())
	}
	version, versionAbort := detectReleasePostMergeConsistentVersion(versionFiles)
	if versionAbort != "" {
		return releasePostMergeAbort(4, versionAbort)
	}

	if diffAbort := checkReleasePostMergeDiffFiles(root, runner, versionFiles); diffAbort != "" {
		return releasePostMergeAbort(5, diffAbort)
	}

	changelogBody, changelogAbort := checkReleasePostMergeChangelogSection(root, version)
	if changelogAbort != "" {
		return releasePostMergeAbort(6, changelogAbort)
	}

	if collisionAbort := checkReleasePostMergeNoExistingTagOrRelease(root, runner, version); collisionAbort != "" {
		return releasePostMergeAbort(7, collisionAbort)
	}

	if taggedAbort := checkReleasePostMergeHeadNotTagged(root, runner); taggedAbort != "" {
		return releasePostMergeAbort(8, taggedAbort)
	}

	featureBranch := ""
	if prNumber != "" {
		featureBranch = lookupReleasePostMergeFeatureBranch(root, runner, prNumber)
	}

	return releasePostMergeResult{
		ok:            true,
		version:       version,
		base:          base,
		featureBranch: featureBranch,
		changelogBody: strings.Join(changelogBody, "\n"),
		versionFiles:  versionFiles,
	}
}

func releasePostMergeAbort(guardrail int, message string) releasePostMergeResult {
	return releasePostMergeResult{ok: false, guardrail: guardrail, message: message}
}

func checkReleasePostMergeCleanWorktree(root string, runner releasePostMergeCommandRunner) string {
	result := runner(root, "git", "status", "--porcelain")
	if result.exitCode != 0 {
		return "could not read git status — is this a git repository?"
	}
	if strings.TrimSpace(result.stdout) != "" {
		return "uncommitted changes detected — commit or stash before rerunning"
	}
	return ""
}

func detectReleasePostMergeBase(root string, runner releasePostMergeCommandRunner) (string, error) {
	if config := strings.TrimSpace(runner(root, "git", "config", "--get", "loaf.release.base").stdout); config != "" {
		return config, nil
	}
	if defaultBranch := strings.TrimSpace(runner(root, "gh", "repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name").stdout); defaultBranch != "" {
		return defaultBranch, nil
	}
	symRef := strings.TrimSpace(runner(root, "git", "symbolic-ref", "refs/remotes/origin/HEAD").stdout)
	if strings.HasPrefix(symRef, "refs/remotes/origin/") {
		return strings.TrimPrefix(symRef, "refs/remotes/origin/"), nil
	}
	return "", fmt.Errorf("Could not auto-detect base branch. Pass --base <ref> explicitly, or set git config loaf.release.base <ref>")
}

func checkReleasePostMergeOnBase(root string, runner releasePostMergeCommandRunner, base string, current string) string {
	if current == base {
		return ""
	}
	result := runner(root, "git", "merge-base", "--is-ancestor", current, base)
	if result.exitCode == 0 {
		return ""
	}
	return fmt.Sprintf("current branch %s is not the base branch %s — checkout %s and rerun", current, base, base)
}

func detectReleasePostMergeConsistentVersion(files []releaseVersionFile) (string, string) {
	if len(files) == 0 {
		return "", "no version files detected at HEAD — cannot verify version match"
	}
	version := files[0].CurrentVersion
	var mismatches []string
	for _, file := range files[1:] {
		if file.CurrentVersion != version {
			mismatches = append(mismatches, fmt.Sprintf("%s reports %s, expected %s from %s", file.RelativePath, file.CurrentVersion, version, files[0].RelativePath))
		}
	}
	if len(mismatches) > 0 {
		return "", "version mismatch in version file(s):\n    " + strings.Join(mismatches, "\n    ")
	}
	return version, ""
}

func checkReleasePostMergeDiffFiles(root string, runner releasePostMergeCommandRunner, versionFiles []releaseVersionFile) string {
	result := runner(root, "git", "diff", "HEAD^", "HEAD", "--name-only")
	if result.exitCode != 0 {
		return "could not read git diff HEAD^ HEAD — is HEAD a merge of multiple commits or the first commit?"
	}
	changed := map[string]bool{}
	for _, line := range strings.Split(result.stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			changed[trimmed] = true
		}
	}
	var versionPaths []string
	hasVersionFile := false
	for _, file := range versionFiles {
		versionPaths = append(versionPaths, file.RelativePath)
		if changed[file.RelativePath] {
			hasVersionFile = true
		}
	}
	hasChangelog := changed["CHANGELOG.md"]
	if !hasChangelog && !hasVersionFile {
		return "release commit is missing both CHANGELOG.md and any version file diffs — this does not look like a release commit"
	}
	if !hasChangelog {
		return "release commit is missing a CHANGELOG.md diff — verify the changelog was updated"
	}
	if !hasVersionFile {
		return fmt.Sprintf("release commit is missing a version-file diff (expected one of: %s)", strings.Join(versionPaths, ", "))
	}
	return ""
}

func checkReleasePostMergeChangelogSection(root string, version string) ([]string, string) {
	path := filepath.Join(root, "CHANGELOG.md")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "CHANGELOG.md not found at HEAD"
		}
		return nil, "could not read CHANGELOG.md"
	}
	section := extractReleasePostMergeChangelogSection(string(body), version)
	if section == nil {
		return nil, fmt.Sprintf("CHANGELOG.md has no `## [%s]` section", version)
	}
	if len(section) == 0 {
		return nil, fmt.Sprintf("CHANGELOG.md `## [%s]` section has no list items", version)
	}
	return section, ""
}

func extractReleasePostMergeChangelogSection(content string, version string) []string {
	lines := strings.Split(content, "\n")
	heading := "## [" + version + "]"
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), heading) {
			start = i
			break
		}
	}
	if start == -1 {
		return nil
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## [") {
			end = i
			break
		}
	}
	raw := lines[start+1 : end]
	for len(raw) > 0 && strings.TrimSpace(raw[0]) == "" {
		raw = raw[1:]
	}
	for len(raw) > 0 && strings.TrimSpace(raw[len(raw)-1]) == "" {
		raw = raw[:len(raw)-1]
	}
	hasItem := false
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			hasItem = true
			break
		}
	}
	if !hasItem {
		return []string{}
	}
	return raw
}

func checkReleasePostMergeNoExistingTagOrRelease(root string, runner releasePostMergeCommandRunner, version string) string {
	tag := "v" + version
	local := runner(root, "git", "tag", "--list", tag)
	if local.exitCode == 0 && strings.TrimSpace(local.stdout) == tag {
		return fmt.Sprintf("tag v%s already exists locally — run `git tag -d v%s` and rerun", version, version)
	}
	remote := runner(root, "git", "ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if remote.exitCode == 0 && strings.TrimSpace(remote.stdout) != "" {
		return fmt.Sprintf("tag v%s already exists on remote — run `git push origin :refs/tags/v%s` and rerun", version, version)
	}
	gh := runner(root, "gh", "release", "view", tag)
	if !gh.notFound && gh.exitCode == 0 {
		return fmt.Sprintf("GH release v%s already exists — visit the release page and delete it manually before rerunning", version)
	}
	return ""
}

func checkReleasePostMergeHeadNotTagged(root string, runner releasePostMergeCommandRunner) string {
	result := runner(root, "git", "tag", "--points-at", "HEAD")
	if result.exitCode != 0 {
		return ""
	}
	for _, line := range strings.Split(result.stdout, "\n") {
		tag := strings.TrimSpace(line)
		if tag != "" {
			return fmt.Sprintf("HEAD is already tagged as %s; this is not a fresh post-merge state", tag)
		}
	}
	return ""
}

func lookupReleasePostMergeFeatureBranch(root string, runner releasePostMergeCommandRunner, prNumber string) string {
	result := runner(root, "gh", "pr", "view", prNumber, "--json", "headRefName", "-q", ".headRefName")
	if result.notFound || result.exitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.stdout)
}

func executeReleasePostMergeActions(root string, ready releasePostMergeResult, runner releasePostMergeCommandRunner, out io.Writer, errOut io.Writer) (releasePostMergeActionResult, error) {
	tag := "v" + ready.version
	result := releasePostMergeActionResult{}

	tagResult := runner(root, "git", "tag", "-s", tag, "-m", "Release "+ready.version)
	if tagResult.exitCode != 0 {
		return result, fmt.Errorf("failed to create tag %s (exit %d)", tag, tagResult.exitCode)
	}
	result.tagged = true
	fmt.Fprintf(out, "    %s Created tag %s\n", ansiGreen("✓"), tag)

	pushResult := runner(root, "git", "push", "origin", tag)
	if pushResult.exitCode != 0 {
		return result, fmt.Errorf("failed to push tag %s (exit %d)", tag, pushResult.exitCode)
	}
	result.pushed = true
	fmt.Fprintf(out, "    %s Pushed tag %s\n", ansiGreen("✓"), tag)

	releaseResult := runner(root, "gh", "release", "create", tag, "--title", tag, "--notes", ready.changelogBody)
	if releaseResult.notFound {
		return result, fmt.Errorf("gh CLI is not installed; cannot create GH release")
	}
	if releaseResult.exitCode != 0 {
		return result, fmt.Errorf("failed to create GH release %s (exit %d)", tag, releaseResult.exitCode)
	}
	result.released = true
	fmt.Fprintf(out, "    %s Created GH release %s\n", ansiGreen("✓"), tag)

	pullResult := runner(root, "git", "pull", "--rebase", "origin", ready.base)
	if pullResult.exitCode == 0 {
		result.pulled = true
		fmt.Fprintf(out, "    %s Pulled latest from origin/%s\n", ansiGreen("✓"), ready.base)
	} else {
		fmt.Fprintf(errOut, "    %s Failed to pull origin/%s — continuing\n", ansiYellow("⚠"), ready.base)
	}

	if ready.featureBranch != "" {
		localDelete := runner(root, "git", "branch", "-d", ready.featureBranch)
		deletedLocal := localDelete.exitCode == 0
		result.deletedLocal = &deletedLocal
		if deletedLocal {
			fmt.Fprintf(out, "    %s Deleted local branch %s\n", ansiGreen("✓"), ready.featureBranch)
		} else {
			fmt.Fprintf(errOut, "    %s Failed to delete local branch %s (may not be fully merged) — continuing\n", ansiYellow("⚠"), ready.featureBranch)
		}

		remoteDelete := runner(root, "git", "push", "origin", "--delete", ready.featureBranch)
		deletedRemote := remoteDelete.exitCode == 0
		result.deletedRemote = &deletedRemote
		if deletedRemote {
			fmt.Fprintf(out, "    %s Deleted remote branch %s\n", ansiGreen("✓"), ready.featureBranch)
		} else {
			fmt.Fprintf(errOut, "    %s Failed to delete remote branch %s — continuing\n", ansiYellow("⚠"), ready.featureBranch)
		}
	}

	return result, nil
}

func defaultReleasePostMergeCommandRunner(root string, name string, args ...string) releasePostMergeCommandResult {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err == nil {
		return releasePostMergeCommandResult{stdout: strings.TrimSpace(string(output)), exitCode: 0}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return releasePostMergeCommandResult{exitCode: 127, notFound: true}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return releasePostMergeCommandResult{stdout: strings.TrimSpace(string(output)), exitCode: exitErr.ExitCode()}
	}
	return releasePostMergeCommandResult{exitCode: 1}
}
