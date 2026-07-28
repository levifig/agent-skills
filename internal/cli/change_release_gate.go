package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// releaseCohortPreflight is the candidate-first gate (TASK-004). Stable
// candidates require every change with matching target_release to be
// materialized, structurally valid, flip-executed, and (once TASK-005 lands)
// receipt-verified. Prerelease candidates bypass the cohort gate.
func releaseCohortPreflight(rootPath string, candidate string, warnings *[]string) error {
	return releaseCohortPreflightWithOutput(rootPath, candidate, commandOutput, warnings)
}

func releaseCohortPreflightWithOutput(rootPath, candidate string, outputCommand changeGitOutput, warnings *[]string) error {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	_, pinned, pinErr := pinEvidenceAtHEAD(rootPath, outputCommand)
	if pinErr != nil {
		return fmt.Errorf("release blocked: cannot pin HEAD for evidence: %w", pinErr)
	}
	outputCommand = pinned
	if err := requireCompleteChangeHistory(rootPath, outputCommand); err != nil {
		return fmt.Errorf("release blocked: cannot confirm complete Change history: %w", err)
	}
	deleted, err := deletedLineageChangesWithOutput(rootPath, outputCommand)
	if err != nil {
		return fmt.Errorf("release blocked: cannot inspect deleted or renamed Change history at HEAD: %w", err)
	}
	if len(deleted) != 0 {
		return fmt.Errorf("release blocked: retained Change deleted or renamed in HEAD ancestry: %s", strings.Join(deleted, ", "))
	}

	nodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return fmt.Errorf("release blocked: cannot inspect committed Changes at HEAD: %w", err)
	}

	if releaseVersionIsPrerelease(candidate) {
		if warnings != nil {
			*warnings = append(*warnings, lowerCohortWarnings(nodes, candidate, rootPath, outputCommand)...)
		}
		return nil
	}

	// Stable candidate: gate byte-equal cohort.
	var blocked []string
	var cohort []changeNode
	for _, node := range nodes {
		if node.TargetRelease == candidate {
			cohort = append(cohort, node)
		}
	}
	sort.Slice(cohort, func(i, j int) bool { return cohort[i].Slug < cohort[j].Slug })

	for _, node := range cohort {
		if node.Layout == changeLayoutLegacy {
			blocked = append(blocked, formatChangeExecutionBlock(node.Slug, candidate, node.Layout, changeExecutionStatus{}, true))
			continue
		}
		report, reportErr := changeCohortStructuralReport(rootPath, node, nodes, outputCommand)
		if reportErr != nil {
			return fmt.Errorf("release blocked: cannot judge structural validity for %q: %w", node.Slug, reportErr)
		}
		folderRel := filepath.ToSlash(node.Folder)
		if len(report.Violations) != 0 {
			blocked = append(blocked, fmt.Sprintf("change %q targets %s but is structurally invalid (%s); run: loaf change check %s",
				node.Slug, candidate, strings.Join(report.Violations, ", "), folderRel))
			continue
		}
		if !report.Executable {
			blocked = append(blocked, fmt.Sprintf("change %q targets %s but is not executable (contract gaps: %s); run: loaf change check %s",
				node.Slug, candidate, strings.Join(report.Gaps, ", "), folderRel))
			continue
		}
		status, err := changeFolderExecuted(rootPath, node.Folder, node.Layout, outputCommand)
		if err != nil {
			return fmt.Errorf("release blocked: cannot derive execution provenance for %q: %w", node.Slug, err)
		}
		if msg := formatChangeExecutionBlock(node.Slug, candidate, node.Layout, status, true); msg != "" {
			blocked = append(blocked, msg)
			continue
		}
		ok, reason, receiptErr := changeReceiptStatus(rootPath, node.Folder, node, outputCommand)
		if receiptErr != nil {
			return fmt.Errorf("release blocked: cannot inspect receipt for %q: %w", node.Slug, receiptErr)
		}
		if !ok {
			blocked = append(blocked, formatChangeReceiptBlock(node.Slug, candidate, reason, node.Folder))
		}
	}

	// Surface retarget events relevant to the candidate.
	events, err := deriveChangeRetargetEvents(rootPath, outputCommand)
	if err == nil && warnings != nil {
		for _, event := range events {
			if event.From == candidate || event.To == candidate {
				*warnings = append(*warnings, fmt.Sprintf("retarget %s: %s → %s at %s (%s)",
					event.Slug, emptyAsNone(event.From), emptyAsNone(event.To), shortSHA(event.Commit), event.Surface))
			}
		}
		*warnings = append(*warnings, lowerCohortWarnings(nodes, candidate, rootPath, outputCommand)...)
	}

	if len(blocked) != 0 {
		return fmt.Errorf("%s", strings.Join(blocked, "; "))
	}
	return nil
}

// changeCohortStructuralReport is the gate's structural tier: the same composite
// `loaf change check` reports — contract evaluation, lineage validation over the
// full loaded node set, and task-hygiene/conversion findings. Executability here
// is contract-section completeness, never checkbox completion: an unchecked task
// on a verified member stays legal descoped work.
func changeCohortStructuralReport(rootPath string, node changeNode, nodes []changeNode, outputCommand changeGitOutput) (changeCheckReport, error) {
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(node.Folder))
	report := evaluateChangeNode(node, "")
	return composeChangeCheckReport(report, rootPath, folderAbs, node, nodes, outputCommand, false, changeTaskContentHEAD)
}

func emptyAsNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}

func shortSHA(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func lowerCohortWarnings(nodes []changeNode, candidate string, rootPath string, outputCommand changeGitOutput) []string {
	cand, ok := parseReleaseSemver(candidate)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var warnings []string
	for _, node := range nodes {
		if node.TargetRelease == "" || node.TargetRelease == candidate {
			continue
		}
		other, ok := parseReleaseSemver(node.TargetRelease)
		if !ok {
			continue
		}
		if !releaseSemverLess(other, cand) {
			continue
		}
		if seen[node.TargetRelease] {
			continue
		}
		// Check if that lower cohort is incomplete.
		incomplete := false
		for _, member := range nodes {
			if member.TargetRelease != node.TargetRelease {
				continue
			}
			if member.Layout == changeLayoutLegacy {
				incomplete = true
				break
			}
			status, err := changeFolderExecuted(rootPath, member.Folder, member.Layout, outputCommand)
			if err != nil || !status.FlipExecuted {
				incomplete = true
				break
			}
		}
		if incomplete {
			seen[node.TargetRelease] = true
			warnings = append(warnings, fmt.Sprintf("incomplete lower cohort target_release %s (warn only; blocks its own cut)", node.TargetRelease))
		}
	}
	sort.Strings(warnings)
	return warnings
}

func releaseSemverLess(a, b releaseSemver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}

// computeReleaseCandidateVersion derives the version the release would cut for
// the given options (candidate-first ordering).
func computeReleaseCandidateVersion(root string, options releaseOptions) (string, error) {
	snap, err := resolveReleaseSnapshot(root, options)
	return snap.Candidate, err
}

// resolveReleaseCandidate is a thin view over resolveReleaseSnapshot for call
// sites that only need the candidate and bump.
func resolveReleaseCandidate(root string, options releaseOptions) (candidate string, bump string, err error) {
	snap, err := resolveReleaseSnapshot(root, options)
	if err != nil {
		return "", "", err
	}
	return snap.Candidate, snap.Bump, nil
}

// resolveReleaseSnapshot is the single derivation shared by the cohort gate and
// every release consumer: one immutable snapshot of version-file state, bump,
// and candidate resolved at invocation start.
func resolveReleaseSnapshot(root string, options releaseOptions) (releaseSnapshot, error) {
	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return releaseSnapshot{}, err
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil {
		return releaseSnapshot{}, err
	}
	if len(versionFiles) == 0 {
		return releaseSnapshot{}, fmt.Errorf("no version files detected")
	}
	current := versionFiles[0].CurrentVersion
	for _, file := range versionFiles {
		if file.CurrentVersion != current {
			return releaseSnapshot{}, fmt.Errorf("inconsistent version files: %s vs %s", current, file.CurrentVersion)
		}
	}
	snap := releaseSnapshot{
		VersionFiles:   versionFiles,
		CurrentVersion: current,
	}
	if options.postMerge {
		// Finalization targets the stable form of the current prepared version.
		parsed, ok := parseReleaseSemver(current)
		if !ok {
			return releaseSnapshot{}, fmt.Errorf("cannot parse current version %q", current)
		}
		snap.Candidate = fmt.Sprintf("%d.%d.%d", parsed.major, parsed.minor, parsed.patch)
		snap.Bump = "release"
		return snap, nil
	}
	bump, err := effectiveReleaseBump(root, options)
	if err != nil {
		return releaseSnapshot{}, err
	}
	if bump == "" {
		// Nothing unreleased: the executor stops before cutting anything, so the
		// candidate is the version the repository already carries.
		snap.Candidate = current
		return snap, nil
	}
	next := bumpReleaseVersion(current, bump)
	if next == "" {
		return releaseSnapshot{}, fmt.Errorf("cannot bump %q with %q", current, bump)
	}
	snap.Bump = bump
	snap.Candidate = next
	return snap, nil
}

// assertReleaseSnapshotStillCurrent re-reads the snapshot's version files and
// blocks when any has drifted from the version the candidate was resolved from.
func assertReleaseSnapshotStillCurrent(root string, snapshot releaseSnapshot) error {
	if snapshot.CurrentVersion == "" {
		return fmt.Errorf("release blocked: release snapshot was not resolved before apply")
	}
	for _, file := range snapshot.VersionFiles {
		fresh, err := loadReleaseVersionFile(root, file.RelativePath, true)
		if err != nil {
			return fmt.Errorf("release blocked: cannot re-read version file %s: %w", file.RelativePath, err)
		}
		if fresh.CurrentVersion != snapshot.CurrentVersion {
			return fmt.Errorf("release blocked: version drifted from %s to %s since preflight; re-run release", snapshot.CurrentVersion, fresh.CurrentVersion)
		}
	}
	return nil
}

// effectiveReleaseBump resolves the bump the release will actually apply: the
// explicit --bump flag when given, otherwise the bump the unreleased commits
// suggest. It returns "" when there is nothing to release. The gate and the
// executor share this derivation so preflight gates the version that gets cut
// instead of the one the repository happens to sit on.
func effectiveReleaseBump(root string, options releaseOptions) (string, error) {
	if options.bump != "" {
		return options.bump, nil
	}
	baseRef, err := releaseCandidateBaseRef(root, options)
	if err != nil {
		return "", err
	}
	return effectiveReleaseBumpFrom(options, releaseCommitsSince(root, baseRef)), nil
}

// effectiveReleaseBumpFrom resolves the same bump from an already-loaded commit
// range, so the executor never re-derives it from different inputs.
func effectiveReleaseBumpFrom(options releaseOptions, commits []releaseCommit) string {
	if options.bump != "" {
		return options.bump
	}
	if len(commits) == 0 {
		return ""
	}
	return suggestReleaseBump(commits)
}

// releaseCandidateBaseRef resolves the commit range the suggested bump reads,
// mirroring the executor: an explicit --base wins, --pre-merge auto-detects its
// base, and everything else measures from the last tag.
func releaseCandidateBaseRef(root string, options releaseOptions) (string, error) {
	base := options.base
	if base == "" && options.preMerge {
		detected, _, err := detectReleaseBase(root)
		if err != nil {
			return "", err
		}
		base = detected
	}
	if base == "" {
		return releaseLastTag(root), nil
	}
	return validateReleaseBaseRef(root, base)
}
