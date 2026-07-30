package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
// Presence uses Lstat and requires a regular file so a dangling or external
// symlink cannot silently disarm the gate by looking absent, or validate
// content that is not retained in the tree.
func checkReleaseCapabilityEvidence(root string) (present bool, err error) {
	path := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
	info, statErr := os.Lstat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return true, fmt.Errorf("inspect capability evidence %s: %w", TargetCapabilityEvidenceRecordPath, statErr)
	}
	if !info.Mode().IsRegular() {
		return true, fmt.Errorf("capability evidence %s is present but not a regular file (symlinks and other irregular files are unusable)", TargetCapabilityEvidenceRecordPath)
	}
	if _, loadErr := LoadTargetCapabilityEvidence(path); loadErr != nil {
		return true, loadErr
	}
	return true, nil
}

func releaseApplyCapabilityEvidenceRefusal(err error) error {
	return fmt.Errorf("Refusing to commit release artifacts: capability evidence is invalid or stale against the rebuilt tree: %v; re-record with the matching runner (%s) after the artifact rebuild (the prepared tree stays in place), then rerun the release — the rerun accepts the release-prepared worktree", err, releaseCapabilityEvidenceRunners)
}

func releasePostMergeCapabilityEvidenceAbortMessage(err error) string {
	return fmt.Sprintf("capability evidence is invalid or stale on the merged tree: %v — re-record against the merged tree, land the receipts as a single evidence-only commit on the base branch, and rerun loaf release --post-merge", err)
}

// releaseDirtyPathsArePreparedOnly reports whether every dirty path is within
// the bounded release-prepared set: version files, CHANGELOG.md, tracked
// generated artifact outputs, the capability registry, and evidence
// receipt/source paths referenced by it.
func releaseDirtyPathsArePreparedOnly(root string, dirtyPaths []string) bool {
	if len(dirtyPaths) == 0 {
		return true
	}
	prepared, err := releasePreparedDirtyPathSet(root)
	if err != nil {
		return false
	}
	return releaseDirtyPathsArePreparedOnlyWithSet(root, dirtyPaths, prepared)
}

func releasePreparedDirtyPathSet(root string) (map[string]bool, error) {
	allowed := map[string]bool{
		"CHANGELOG.md":                     true,
		TargetCapabilityEvidenceRecordPath: true,
	}
	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return nil, err
	}
	versionFiles, err := detectReleaseVersionFiles(root, configOverrides)
	if err != nil {
		return nil, err
	}
	for _, file := range versionFiles {
		allowed[filepath.ToSlash(file.RelativePath)] = true
	}
	// Version-file detection reads worktree content; also admit the common
	// ecosystem version paths so a partially-prepared tree still classifies.
	for _, candidate := range []string{"package.json", "pyproject.toml", "Cargo.toml", ".agents/loaf.json", ".claude-plugin/marketplace.json"} {
		allowed[candidate] = true
	}
	for _, path := range releaseCapabilityEvidenceReferencedPaths(root) {
		allowed[path] = true
	}
	return allowed, nil
}

// releasePathMatchesPreparedArtifact reports whether path is under the tracked
// generated-output trees the artifact commands rewrite.
func releasePathMatchesPreparedArtifact(path string) bool {
	path = filepath.ToSlash(path)
	return evidencePathExcluded(path, releasePreparedArtifactGlobs)
}

// releaseCapabilityEvidenceReferencedPaths returns the registry path plus every
// evidence source path it names (anchors stripped). Best-effort: a missing or
// unreadable registry yields only the registry path itself when present on disk.
func releaseCapabilityEvidenceReferencedPaths(root string) []string {
	paths := map[string]bool{TargetCapabilityEvidenceRecordPath: true}
	registryPath := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
	info, err := os.Lstat(registryPath)
	if err != nil || !info.Mode().IsRegular() {
		return sortedKeys(paths)
	}
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return sortedKeys(paths)
	}
	var contract struct {
		Records []struct {
			Context struct {
				Modes []struct {
					Evidence struct {
						Source string `json:"source"`
					} `json:"evidence"`
				} `json:"modes"`
			} `json:"context"`
			Completion struct {
				Evidence struct {
					Source string `json:"source"`
				} `json:"evidence"`
			} `json:"completion"`
		} `json:"records"`
	}
	if err := json.Unmarshal(data, &contract); err != nil {
		return sortedKeys(paths)
	}
	addSource := func(source string) {
		relative, err := safeEvidenceRelativePath(source)
		if err != nil {
			return
		}
		paths[filepath.ToSlash(relative)] = true
	}
	for _, record := range contract.Records {
		for _, mode := range record.Context.Modes {
			addSource(mode.Evidence.Source)
		}
		addSource(record.Completion.Evidence.Source)
	}
	return sortedKeys(paths)
}

// releaseIsEvidenceOnlyPath reports whether path is the capability registry or
// an evidence source it references — the only paths an evidence-repair commit
// may touch.
func releaseIsEvidenceOnlyPath(root, path string) bool {
	path = filepath.ToSlash(path)
	for _, allowed := range releaseCapabilityEvidenceReferencedPaths(root) {
		if path == allowed {
			return true
		}
	}
	return false
}

// releaseIsEvidenceOnlyRepairCommit reports whether HEAD is a single
// evidence-only repair commit sitting directly atop a release commit. Depth is
// exactly one — history is not scanned.
func releaseIsEvidenceOnlyRepairCommit(root string, runner releasePostMergeCommandRunner) bool {
	parent := runner(root, "git", "rev-parse", "--verify", "HEAD^")
	if parent.exitCode != 0 || strings.TrimSpace(parent.stdout) == "" {
		return false
	}
	diff := runner(root, "git", "diff", "HEAD^", "HEAD", "--name-only")
	if diff.exitCode != 0 {
		return false
	}
	var paths []string
	for _, line := range strings.Split(diff.stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			paths = append(paths, filepath.ToSlash(trimmed))
		}
	}
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !releaseIsEvidenceOnlyPath(root, path) {
			return false
		}
	}
	return true
}

// releasePreparedDirtyPathAllowed is the per-path predicate used by the apply
// clean-worktree gate: explicit prepared-set membership or a generated-output
// artifact path.
func releasePreparedDirtyPathAllowed(root, path string, prepared map[string]bool) bool {
	path = filepath.ToSlash(path)
	if prepared[path] {
		return true
	}
	return releasePathMatchesPreparedArtifact(path)
}

func releaseDirtyPathsArePreparedOnlyWithSet(root string, dirtyPaths []string, prepared map[string]bool) bool {
	if len(dirtyPaths) == 0 {
		return true
	}
	for _, path := range dirtyPaths {
		if !releasePreparedDirtyPathAllowed(root, path, prepared) {
			return false
		}
	}
	return true
}
