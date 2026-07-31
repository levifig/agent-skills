package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// releaseCapabilityEvidenceRunners is static remediation copy: all three
// receipt runners share the --client/--expected-version/--receipt shape. The
// list is never parsed out of the loader error.
const releaseCapabilityEvidenceRunners = "cli/scripts/smoke-claude-code-startup.mjs, cli/scripts/smoke-codex-startup.mjs, or cli/scripts/smoke-opencode-request-context.mjs, each with --client <cli> --expected-version <installed> --receipt <path>"

// releasePreparedArtifactGlobs are tracked outputs the release artifact
// commands rewrite. Reuses the same component-anchored glob grammar as
// ReleaseMetadataAllowlist / evidencePathExcluded — not a broader allowlist.
var releasePreparedArtifactGlobs = []string{
	"dist/**",
	"plugins/**",
	"bin/**",
	".claude-plugin/**",
}

// checkReleaseCapabilityEvidence validates the capability evidence registry
// against the tree at root. Absent evidence exempts the project (present is
// false); any other failure — unreadable, invalid, irregular, or stale
// receipts — must refuse the release. There is deliberately no override.
//
// Presence walks every path component from the repository root with Lstat:
// intermediate components must be real directories (not symlinks), and the leaf
// must be a regular file. A symlinked component is present-but-unusable, never
// absent — so a dangling or external symlink cannot silently disarm the gate.
func checkReleaseCapabilityEvidence(root string) (present bool, err error) {
	path, probeErr := probeCapabilityEvidenceRegistryPath(root)
	if probeErr != nil {
		return true, probeErr
	}
	if path == "" {
		return false, nil
	}
	if _, loadErr := LoadTargetCapabilityEvidence(path); loadErr != nil {
		return true, loadErr
	}
	return true, nil
}

// probeCapabilityEvidenceRegistryPath component-walks root → registry. Returns
// ("", nil) when any component is missing (absent), a non-empty path when the
// leaf is a regular file, and an error when a component is present but unusable
// (symlink, non-directory intermediate, non-regular leaf).
func probeCapabilityEvidenceRegistryPath(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("inspect capability evidence %s: resolve root: %w", TargetCapabilityEvidenceRecordPath, err)
	}
	relative := filepath.FromSlash(TargetCapabilityEvidenceRecordPath)
	current := absRoot
	components := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for index, component := range components {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return "", nil
			}
			return "", fmt.Errorf("inspect capability evidence %s: %w", TargetCapabilityEvidenceRecordPath, statErr)
		}
		isLast := index == len(components)-1
		if info.Mode()&os.ModeSymlink != 0 {
			if isLast {
				return "", fmt.Errorf("capability evidence %s is present but not a regular file (symlinks and other irregular files are unusable)", TargetCapabilityEvidenceRecordPath)
			}
			return "", fmt.Errorf("capability evidence %s is present but unusable: path component %q is a symlink", TargetCapabilityEvidenceRecordPath, filepath.ToSlash(strings.TrimPrefix(current, absRoot+string(filepath.Separator))))
		}
		if isLast {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("capability evidence %s is present but not a regular file (symlinks and other irregular files are unusable)", TargetCapabilityEvidenceRecordPath)
			}
			return current, nil
		}
		if !info.IsDir() {
			return "", fmt.Errorf("capability evidence %s is present but unusable: path component %q is not a directory", TargetCapabilityEvidenceRecordPath, filepath.ToSlash(strings.TrimPrefix(current, absRoot+string(filepath.Separator))))
		}
	}
	return "", nil
}

func releaseApplyCapabilityEvidenceRefusal(err error) error {
	return fmt.Errorf("Refusing to commit release artifacts: capability evidence is invalid or stale against the rebuilt tree: %v; re-record with the matching runner (%s) after the artifact rebuild (the prepared tree stays in place — version files remain at the candidate so runners can hash candidate-versioned artifacts; CHANGELOG.md is restored to HEAD), then rerun the release — the rerun accepts the release-prepared worktree", err, releaseCapabilityEvidenceRunners)
}

func releasePostMergeCapabilityEvidenceAbortMessage(err error) string {
	return fmt.Sprintf("capability evidence is invalid or stale on the merged tree: %v — re-record against the merged tree, land the receipts as a single evidence-only commit on the base branch, and rerun loaf release --post-merge", err)
}

// releasePathMatchesPreparedArtifact reports whether path is under the tracked
// generated-output trees the artifact commands rewrite.
func releasePathMatchesPreparedArtifact(path string) bool {
	path = filepath.ToSlash(path)
	return evidencePathExcluded(path, releasePreparedArtifactGlobs)
}

// releaseGitShowPath returns the blob at rev:relPath (slash-separated) without
// trimming the body. Missing paths return an error.
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

// releaseCapabilityEvidenceAllowlist is the union of evidence source paths named
// by the HEAD registry and the worktree registry, plus the registry path itself.
// Schema-only DecodeTargetCapabilityEvidence is enough; full load still runs as
// the gate later. Unreferenced research/ files are not admitted.
func releaseCapabilityEvidenceAllowlist(root string) map[string]bool {
	paths := map[string]bool{TargetCapabilityEvidenceRecordPath: true}
	addJSON := func(data []byte) {
		for _, path := range releaseCapabilityEvidenceSourcePathsFromJSON(data) {
			paths[path] = true
		}
	}
	if data, err := releaseGitShowPath(root, "HEAD", TargetCapabilityEvidenceRecordPath); err == nil {
		addJSON(data)
	}
	registryPath := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
	if data, err := os.ReadFile(registryPath); err == nil {
		addJSON(data)
	}
	return paths
}

// releaseCapabilityEvidenceSourcePathsFromJSON extracts every evidence source
// path from a registry document via the typed decoder. It does not include the
// registry path itself. Invalid documents yield no sources.
func releaseCapabilityEvidenceSourcePathsFromJSON(data []byte) []string {
	contract, err := DecodeTargetCapabilityEvidence(data)
	if err != nil {
		return nil
	}
	paths := map[string]bool{}
	addEvidence := func(evidence TargetCapabilityEvidenceRecord) {
		relative, err := safeEvidenceRelativePath(evidence.Source)
		if err != nil {
			return
		}
		paths[filepath.ToSlash(relative)] = true
	}
	for _, record := range contract.Records {
		for _, mode := range record.Context.Modes {
			addEvidence(mode.Evidence)
		}
		addEvidence(record.Completion.Evidence)
	}
	return sortedKeys(paths)
}

// releaseCapabilityEvidenceInstalledSmokePathsFromJSON returns source paths of
// evidence entries whose level is installed-smoke (receipts only).
func releaseCapabilityEvidenceInstalledSmokePathsFromJSON(data []byte) []string {
	contract, err := DecodeTargetCapabilityEvidence(data)
	if err != nil {
		return nil
	}
	paths := map[string]bool{}
	addSmoke := func(evidence TargetCapabilityEvidenceRecord) {
		if evidence.Level != "installed-smoke" {
			return
		}
		relative, err := safeEvidenceRelativePath(evidence.Source)
		if err != nil {
			return
		}
		paths[filepath.ToSlash(relative)] = true
	}
	for _, record := range contract.Records {
		for _, mode := range record.Context.Modes {
			addSmoke(mode.Evidence)
		}
		addSmoke(record.Completion.Evidence)
	}
	return sortedKeys(paths)
}

// releaseIsEvidenceOnlyPath reports whether path is the capability registry or
// a source it references (HEAD ∪ worktree). Used for docs/changes residual
// filtering during apply. Repair classification uses the parent-commit
// installed-smoke paths instead.
func releaseIsEvidenceOnlyPath(root, path string) bool {
	path = filepath.ToSlash(path)
	return releaseCapabilityEvidenceAllowlist(root)[path]
}

// releaseParseNameStatusZ parses `git diff --name-status --no-renames -z`
// output. Paths are taken raw (no TrimSpace). Only added/modified statuses are
// accepted; rename/copy/type-change/delete and any other status refuse the
// parse so the caller treats the commit as not receipt-only.
func releaseParseNameStatusZ(raw string) (paths []string, ok bool) {
	if raw == "" {
		return nil, false
	}
	parts := strings.Split(raw, "\x00")
	for i := 0; i < len(parts); {
		if parts[i] == "" {
			i++
			continue
		}
		status := parts[i]
		i++
		if i >= len(parts) {
			return nil, false
		}
		path := parts[i]
		i++
		if path == "" || status == "" {
			return nil, false
		}
		// With --no-renames, status is a single letter. Reject anything except A/M
		// (rename/copy/type-change/delete are not receipt-only repairs).
		if len(status) != 1 || (status[0] != 'A' && status[0] != 'M') {
			return nil, false
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	if len(paths) == 0 {
		return nil, false
	}
	return paths, true
}

// releaseIsEvidenceOnlyRepairCommit reports whether HEAD is a single
// receipt-only repair commit sitting directly atop a release commit. Depth is
// exactly one — history is not scanned.
//
// Allowed paths are installed-smoke sources from the PARENT commit's registry
// (not HEAD's), and the registry file itself must be unchanged. A repair that
// edits the registry or a non-receipt source (fixture/source level) is not
// receipt-only; recovery is redoing the release PR.
func releaseIsEvidenceOnlyRepairCommit(root string, runner releasePostMergeCommandRunner) bool {
	parent := runner(root, "git", "rev-parse", "--verify", "HEAD^")
	if parent.exitCode != 0 || strings.TrimSpace(parent.stdout) == "" {
		return false
	}
	diff := runner(root, "git", "diff", "--name-status", "--no-renames", "-z", "HEAD^", "HEAD")
	if diff.exitCode != 0 {
		return false
	}
	raw := diff.rawOutput()
	paths, ok := releaseParseNameStatusZ(raw)
	if !ok {
		return false
	}
	for _, path := range paths {
		if path == TargetCapabilityEvidenceRecordPath {
			return false
		}
	}
	show := runner(root, "git", "show", "HEAD^:"+TargetCapabilityEvidenceRecordPath)
	if show.exitCode != 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, path := range releaseCapabilityEvidenceInstalledSmokePathsFromJSON([]byte(show.rawOutput())) {
		allowed[path] = true
	}
	if len(allowed) == 0 {
		return false
	}
	for _, path := range paths {
		if !allowed[path] {
			return false
		}
	}
	return true
}

// releaseVersionPathSet returns relative paths of version files the release
// may bump, used to classify dirt on resume.
func releaseVersionPathSet(root string, options releaseOptions) map[string]bool {
	allowed := map[string]bool{}
	for _, candidate := range []string{"package.json", "pyproject.toml", "Cargo.toml", ".agents/loaf.json", ".claude-plugin/marketplace.json"} {
		allowed[candidate] = true
	}
	configOverrides, err := releaseConfigVersionFiles(root)
	if err == nil {
		for _, path := range configOverrides {
			allowed[filepath.ToSlash(path)] = true
		}
	}
	for _, path := range options.versionFile {
		allowed[filepath.ToSlash(path)] = true
	}
	// Prefer detecting actual files so partial fixtures still classify.
	overrides := options.versionFile
	if len(overrides) == 0 {
		overrides = configOverrides
	}
	if files, err := detectReleaseVersionFiles(root, overrides); err == nil {
		for _, file := range files {
			allowed[filepath.ToSlash(file.RelativePath)] = true
		}
	}
	// Also admit paths present at HEAD even when worktree content is odd.
	for path := range allowed {
		if _, err := releaseGitShowPath(root, "HEAD", path); err == nil {
			allowed[path] = true
		}
	}
	return allowed
}

// releaseRenderVersionContent re-derives the on-disk body for a version file
// after bumping currentVersion → candidate, matching prepareReleaseVersionUpdates.
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

// releaseGitHeadBlobMode returns the git object mode for relPath at HEAD
// (e.g. "100644", "100755"). Empty or non-blob entries error.
func releaseGitHeadBlobMode(root, relPath string) (string, error) {
	cmd := exec.Command("git", "ls-tree", "HEAD", "--", filepath.ToSlash(relPath))
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", fmt.Errorf("no ls-tree entry for %s at HEAD", relPath)
	}
	// "100644 blob <hash>\t<path>" — mode is the first field.
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return "", fmt.Errorf("parse ls-tree entry %q", line)
	}
	return fields[0], nil
}

// releaseWorktreeBlobMode maps a regular worktree file's mode onto git's
// blob mode alphabet (100644 vs 100755). Non-regular paths return "".
func releaseWorktreeBlobMode(info os.FileInfo) string {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ""
	}
	if info.Mode().Perm()&0o111 != 0 {
		return "100755"
	}
	return "100644"
}

// releaseVersionFileMatchesCandidate reports whether the worktree path is a
// regular file whose git mode matches HEAD and whose bytes equal the candidate
// rendering derived in memory from HEAD content + candidate. Symlinks and other
// non-regular paths are never admitted (os.Lstat; ReadFile must not follow).
func releaseVersionFileMatchesCandidate(root, relPath, candidate string) bool {
	if candidate == "" {
		return false
	}
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	headMode, err := releaseGitHeadBlobMode(root, relPath)
	if err != nil || headMode == "" {
		return false
	}
	if releaseWorktreeBlobMode(info) != headMode {
		return false
	}
	headBody, err := releaseGitShowPath(root, "HEAD", relPath)
	if err != nil {
		return false
	}
	currentVersion, format, err := parseReleaseVersion(relPath, headBody)
	if err != nil {
		return false
	}
	expected, err := releaseRenderVersionContent(relPath, headBody, currentVersion, format, candidate)
	if err != nil {
		return false
	}
	actual, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return bytes.Equal(actual, []byte(expected))
}

// releaseResolveCandidateForClassification returns the candidate version this
// mutating invocation will cut, using HEAD version baselines so a refused
// prepare's worktree bumps do not shift the candidate.
func releaseResolveCandidateForClassification(root string, options releaseOptions) string {
	if options.snapshot.Candidate != "" {
		return options.snapshot.Candidate
	}
	snap, err := resolveReleaseSnapshot(root, options)
	if err != nil {
		return ""
	}
	return snap.Candidate
}

// releaseRestoreGeneratedFromHEAD hard-restores only tracked generated-output
// dirt from HEAD. Version files, CHANGELOG, and evidence paths are never
// checkout'd by classification alone.
func releaseRestoreGeneratedFromHEAD(root string, entries []releaseStatusEntry) error {
	var toRestore []string
	for _, entry := range entries {
		if entry.untracked || entry.deleted {
			continue
		}
		path := filepath.ToSlash(entry.path)
		if releasePathMatchesPreparedArtifact(path) {
			toRestore = append(toRestore, path)
		}
	}
	if len(toRestore) == 0 {
		return nil
	}
	args := append([]string{"checkout", "HEAD", "--"}, toRestore...)
	if err := releaseCommandRun(root, "git", args...); err != nil {
		return fmt.Errorf("Refusing to prepare release: cannot restore generated outputs from HEAD: %w", err)
	}
	return nil
}

// releaseRestoreChangelogFromHEAD undoes the changelog insertion this run wrote
// when the evidence gate refuses. Version files and generated outputs stay.
func releaseRestoreChangelogFromHEAD(root string) error {
	if releaseCommandOK(root, "git", "cat-file", "-e", "HEAD:CHANGELOG.md") {
		if err := releaseCommandRun(root, "git", "checkout", "HEAD", "--", "CHANGELOG.md"); err != nil {
			return fmt.Errorf("cannot restore CHANGELOG.md from HEAD: %w", err)
		}
		return nil
	}
	// This run created CHANGELOG.md; remove it.
	path := filepath.Join(root, "CHANGELOG.md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove CHANGELOG.md created by this run: %w", err)
	}
	return nil
}

func releaseClassicCleanWorktreeRefusal(dirtyPaths []string) error {
	return fmt.Errorf("Refusing to prepare release: mutating release modes require a clean unignored worktree; changelog curation belongs on a release branch in the --pre-merge flow — commit, stash, or remove: %s", strings.Join(dirtyPaths, ", "))
}
