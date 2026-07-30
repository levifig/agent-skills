package cli

import (
	"fmt"
	"io"
	"strings"
)

// releaseFlowAdvisoryText names the sanctioned release-PR flow at the decision point where a direct mutating invocation is about to skip it. Advisory only, never blocking.
const releaseFlowAdvisoryText = "releases are prepared on a release branch: loaf release --pre-merge there, squash-merge the release PR, then loaf release --post-merge here; PR CI verifies the prepared tree so evidence canaries surface before tagging. Proceeding directly skips that."

// resolveReleaseDefaultBranch resolves the repository default branch from local git state only — refs/remotes/origin/HEAD when present, then a local main or master branch. A release ceremony must not add a network dependency to print advice.
func resolveReleaseDefaultBranch(root string) string {
	if symRef := releaseCommandOutput(root, "git", "symbolic-ref", "refs/remotes/origin/HEAD"); strings.HasPrefix(symRef, "refs/remotes/origin/") {
		return strings.TrimPrefix(symRef, "refs/remotes/origin/")
	}
	for _, candidate := range []string{"main", "master"} {
		if releaseCommandOK(root, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+candidate) {
			return candidate
		}
	}
	return ""
}

// releaseInvocationWantsFlowAdvisory reports whether this invocation is a mutating release mode (interactive or --bump) on the repository default branch outside the two-phase flags. Read-only modes and the two-phase flags never advise.
func releaseInvocationWantsFlowAdvisory(root string, options releaseOptions) bool {
	if options.help || options.dryRun || options.preMerge || options.postMerge {
		return false
	}
	defaultBranch := resolveReleaseDefaultBranch(root)
	if defaultBranch == "" {
		return false
	}
	return releaseCommandOutput(root, "git", "symbolic-ref", "--short", "HEAD") == defaultBranch
}

// printReleaseFlowAdvisory emits the advisory as one short paragraph before the analysis phase.
func printReleaseFlowAdvisory(out io.Writer) {
	fmt.Fprintf(out, "  %s %s\n\n", ansiYellow("advisory:"), releaseFlowAdvisoryText)
}
