package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func (r Runner) runRelease(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) > 0 {
		switch args[0] {
		case "suggest":
			return r.runReleaseSuggest(args[1:], out, runtimeRoot)
		case "cut":
			return r.runReleaseCut(args[1:], out, runtimeRoot)
		}
	}
	options, err := parseReleaseArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeReleaseHelp(out)
		return nil
	}
	// Print the flow advisory before candidate analysis so a blocked mutating
	// invocation still names the sanctioned door.
	if releaseInvocationWantsFlowAdvisory(runtimeRoot, options) {
		printReleaseFlowAdvisory(out)
	}
	// Apply-path resume classifies prepared dirt (verify-then-restore): admit
	// candidate-matching version files, refuse dirty CHANGELOG, restore only
	// generated outputs from HEAD. Dry-run and post-merge do not mutate.
	if !options.dryRun && !options.postMerge {
		if err := requireReleaseCleanWorktree(runtimeRoot, options); err != nil {
			return err
		}
	}
	snapshot, err := resolveReleaseSnapshot(runtimeRoot, options)
	if err != nil {
		return fmt.Errorf("release blocked: cannot compute candidate version: %w", err)
	}
	options.snapshot = snapshot
	if options.dryRun {
		errOut := r.Stderr
		if errOut == nil {
			errOut = os.Stderr
		}
		return runReleaseDryRun(runtimeRoot, options, out, errOut)
	}
	if !options.postMerge {
		errOut := r.Stderr
		if errOut == nil {
			errOut = os.Stderr
		}
		return runReleaseApply(runtimeRoot, options, firstReader(r.Stdin, os.Stdin), out, errOut)
	}
	errOut := r.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	return runReleasePostMerge(runtimeRoot, options.snapshot, out, errOut)
}

func releaseAllowsPrereleaseLineageBypass(root string, options releaseOptions) bool {
	// Retained for tests that assert the old predicate; the live path uses
	// resolveReleaseSnapshot instead.
	if options.postMerge {
		if options.bump != "" {
			return false
		}
	} else if options.bump != "prerelease" {
		return false
	}
	configOverrides, err := releaseConfigVersionFiles(root)
	if err != nil {
		return false
	}
	versionOverrides := options.versionFile
	if len(versionOverrides) == 0 {
		versionOverrides = configOverrides
	}
	versionFiles, err := detectReleaseVersionFiles(root, versionOverrides)
	if err != nil || len(versionFiles) == 0 {
		return false
	}
	currentVersion := versionFiles[0].CurrentVersion
	for _, file := range versionFiles {
		if file.CurrentVersion != currentVersion {
			return false
		}
		version, ok := parseReleaseSemver(file.CurrentVersion)
		if !ok || version.prerelease == "" {
			return false
		}
	}
	return true
}

func writeReleaseHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf release [options]",
		"",
		"Create a new release with changelog, version bump, and tag.",
		"",
		"Options:",
		"  --dry-run              Preview release without making changes",
		"  --bump <type>          Skip interactive bump choice; --bump release finalizes the stable target, --post-merge publishes the prepared version",
		"  --base <ref>           Use commits since <ref> instead of last tag",
		"  --no-tag               Skip git tag creation",
		"  --tag                  Force git tag creation",
		"  --no-gh                Skip GitHub release draft",
		"  --gh                   Force GitHub release draft",
		"  --version-file <path>  Override version file path (repeatable)",
		"  --pre-merge            Prepare release artifacts before squash-merge",
		"  --post-merge           Finalize release after squash-merge",
		"  -y, --yes              Skip confirmation prompt",
		"  -h, --help             Show help",
		"",
		"Retroactive track (does not run the legacy flag path):",
		"  loaf release suggest   Report landed work since the last tag",
		"  loaf release cut       Record a release from landed work",
	}, "\n"))
}
