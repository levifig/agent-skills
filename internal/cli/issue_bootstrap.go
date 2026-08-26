package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/levifig/loaf/internal/state"
)

func writeIssueBootstrapHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue bootstrap [--json]", "Steer issue authority toward Linear when a tracker is configured, or emit trackerless branch:/pr: guidance.",
		"--json  Output bootstrap result as JSON")
}

func (r Runner) runIssueBootstrap(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("unknown issue bootstrap option %q", arg)
		}
	}
	projectRoot, err := r.requireIssueSQLiteState("issue bootstrap", runtime)
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	result, err := state.BootstrapTrackerSteering(context.Background(), projectRoot, resolver)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "mode: %s\nauthority: %s\n", result.Mode, result.Authority)
	if result.Prefix != "" {
		fmt.Fprintf(out, "prefix: %s\n", result.Prefix)
	}
	fmt.Fprintf(out, "guidance: %s\n", result.Guidance)
	return nil
}
