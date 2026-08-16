package cli

import (
	"fmt"
	"io"
	"strings"
)

func (r Runner) runRelease(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 || isHelpArg(args) {
		writeReleaseHelp(out)
		if len(args) == 0 {
			return fmt.Errorf("release requires a subcommand; use loaf release suggest or loaf release cut")
		}
		return nil
	}
	switch args[0] {
	case "suggest":
		return r.runReleaseSuggest(args[1:], out, runtimeRoot)
	case "cut":
		return r.runReleaseCut(args[1:], out, runtimeRoot)
	default:
		writeReleaseHelp(out)
		return fmt.Errorf("unknown release invocation %q; use loaf release suggest or loaf release cut", args[0])
	}
}

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

func writeReleaseHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf release <subcommand> [options]",
		"",
		"Cut a retroactive release from already-landed work. The only shipping path is suggest then cut.",
		"",
		"Subcommands:",
		"  suggest   Report landed work since the last version tag",
		"  cut       Record a release from landed work",
		"",
		"The legacy flag path (--bump, --pre-merge, --post-merge, --yes) has been removed.",
		"Use loaf release suggest and loaf release cut.",
	}, "\n"))
}
