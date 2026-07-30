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

// releaseEvidenceSourceTreeGlob is the path class for capability evidence
// receipts and narrative sources under Change research trees. Untracked
// re-record files are tolerated only under this tree; a renamed receipt lands
// here before the registry points at it.
const releaseEvidenceSourceTreeGlob = "docs/changes/*/research/**"

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
	return fmt.Errorf("Refusing to commit release artifacts: capability evidence is invalid or stale against the rebuilt tree: %v; re-record with the matching runner (%s) after the artifact rebuild (the prepared tree stays in place), then rerun the release — the rerun accepts the release-prepared worktree", err, releaseCapabilityEvidenceRunners)
}

func releasePostMergeCapabilityEvidenceAbortMessage(err error) string {
	return fmt.Sprintf("capability evidence is invalid or stale on the merged tree: %v — re-record against the merged tree, land the receipts as a single evidence-only commit on the base branch, and rerun loaf release --post-merge", err)
}

// releaseDirtyPathsArePreparedOnly reports whether every dirty path is within
// the bounded release-prepared set: version files, CHANGELOG.md, tracked
// generated artifact outputs, the capability registry, and evidence
// receipt/source paths under docs/changes/*/research/.
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

// releasePathMatchesEvidenceSourceTree reports whether path sits under a Change
// research tree that holds capability evidence sources and receipts.
func releasePathMatchesEvidenceSourceTree(path string) bool {
	return matchEvidenceGlob(filepath.ToSlash(path), releaseEvidenceSourceTreeGlob)
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
	for _, path := range releaseCapabilityEvidenceSourcePathsFromJSON(data) {
		paths[path] = true
	}
	return sortedKeys(paths)
}

// releaseCapabilityEvidenceSourcePathsFromJSON extracts evidence source paths
// from a registry document. It does not include the registry path itself.
func releaseCapabilityEvidenceSourcePathsFromJSON(data []byte) []string {
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
		return nil
	}
	paths := map[string]bool{}
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

// releaseIsEvidenceClassPath reports whether path is the capability registry or
// an evidence source/receipt path. Resume restores every other prepared path
// from HEAD and leaves this class alone so re-recorded receipts survive.
func releaseIsEvidenceClassPath(root, path string) bool {
	path = filepath.ToSlash(path)
	if path == TargetCapabilityEvidenceRecordPath {
		return true
	}
	if releasePathMatchesEvidenceSourceTree(path) {
		return true
	}
	for _, allowed := range releaseCapabilityEvidenceReferencedPaths(root) {
		if path == allowed {
			return true
		}
	}
	return false
}

// releaseIsEvidenceOnlyPath reports whether path is the capability registry or
// an evidence source it references — used for docs/changes residual filtering
// during apply. Repair classification uses the parent-commit registry instead.
func releaseIsEvidenceOnlyPath(root, path string) bool {
	path = filepath.ToSlash(path)
	if releasePathMatchesEvidenceSourceTree(path) {
		return true
	}
	for _, allowed := range releaseCapabilityEvidenceReferencedPaths(root) {
		if path == allowed {
			return true
		}
	}
	return false
}

// releaseIsEvidenceOnlyRepairCommit reports whether HEAD is a single
// receipt-only repair commit sitting directly atop a release commit. Depth is
// exactly one — history is not scanned.
//
// Allowed paths are derived from the PARENT commit's registry (not HEAD's), and
// the registry file itself must be unchanged. A repair that edits the registry
// is not receipt-only; recovery is redoing the release PR.
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
		if path == TargetCapabilityEvidenceRecordPath {
			return false
		}
	}
	show := runner(root, "git", "show", "HEAD^:"+TargetCapabilityEvidenceRecordPath)
	if show.exitCode != 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, path := range releaseCapabilityEvidenceSourcePathsFromJSON([]byte(show.stdout)) {
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

// releasePreparedDirtyPathAllowed is the per-path predicate used by the apply
// clean-worktree gate: explicit prepared-set membership, a generated-output
// artifact path, or an evidence source/receipt under a research tree.
func releasePreparedDirtyPathAllowed(root, path string, prepared map[string]bool) bool {
	path = filepath.ToSlash(path)
	if prepared[path] {
		return true
	}
	if releasePathMatchesPreparedArtifact(path) {
		return true
	}
	return releasePathMatchesEvidenceSourceTree(path)
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

// releaseRestorePreparedBaseline hard-restores tracked dirty files in the
// version / changelog / generated classes from HEAD so resume re-runs the
// normal prepare flow. Evidence-class paths (registry + receipts/sources) are
// left in place — they are the operator's intentional re-record input.
func releaseRestorePreparedBaseline(root string, entries []releaseStatusEntry) error {
	var toRestore []string
	for _, entry := range entries {
		if entry.untracked {
			continue
		}
		path := filepath.ToSlash(entry.path)
		if releaseIsEvidenceClassPath(root, path) {
			continue
		}
		toRestore = append(toRestore, path)
	}
	if len(toRestore) == 0 {
		return nil
	}
	args := append([]string{"checkout", "HEAD", "--"}, toRestore...)
	if err := releaseCommandRun(root, "git", args...); err != nil {
		return fmt.Errorf("Refusing to prepare release: cannot restore prepared baseline from HEAD: %w", err)
	}
	return nil
}
