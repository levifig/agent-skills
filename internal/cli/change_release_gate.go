package cli

import (
	"fmt"
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
		report := evaluateChangeNode(node, "")
		if len(report.Violations) != 0 {
			blocked = append(blocked, fmt.Sprintf("change %q targets %s but is structurally invalid", node.Slug, candidate))
			continue
		}
		status, err := changeFolderExecuted(rootPath, node.Folder, node.Layout, outputCommand)
		if err != nil {
			return fmt.Errorf("release blocked: cannot derive execution provenance for %q: %w", node.Slug, err)
		}
		if msg := formatChangeExecutionBlock(node.Slug, candidate, node.Layout, status, true); msg != "" {
			blocked = append(blocked, msg)
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
	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return "", err
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil {
		return "", err
	}
	if len(versionFiles) == 0 {
		return "", fmt.Errorf("no version files detected")
	}
	current := versionFiles[0].CurrentVersion
	for _, file := range versionFiles {
		if file.CurrentVersion != current {
			return "", fmt.Errorf("inconsistent version files: %s vs %s", current, file.CurrentVersion)
		}
	}
	if options.postMerge {
		// Finalization targets the stable form of the current prepared version.
		parsed, ok := parseReleaseSemver(current)
		if !ok {
			return "", fmt.Errorf("cannot parse current version %q", current)
		}
		return fmt.Sprintf("%d.%d.%d", parsed.major, parsed.minor, parsed.patch), nil
	}
	bump := options.bump
	if bump == "" {
		// Interactive / unspecified: treat as stable-intent only when current is
		// already stable; otherwise require an explicit bump for gating.
		if releaseVersionIsPrerelease(current) {
			return current, nil // stay on prerelease candidate until bump chosen
		}
		return current, nil
	}
	next := bumpReleaseVersion(current, bump)
	if next == "" {
		return "", fmt.Errorf("cannot bump %q with %q", current, bump)
	}
	return next, nil
}
