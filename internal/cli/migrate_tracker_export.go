package cli

import (
	"context"
	"io"

	"github.com/levifig/loaf/internal/state"
)

func writeMigrateTrackerExportHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate tracker-export [--json]", "Emit a versioned, provider-neutral packet containing only open, non-archived issues for harness-native tracker migration.",
		"--json       Output the migration packet as JSON (default)")
}

func (r Runner) runMigrateTrackerExport(args []string, out io.Writer, runtime state.Runtime) error {
	if _, err := parseJSONOnly(args); err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("migrate tracker-export", runtime)
	if err != nil {
		return err
	}
	packet, err := state.ExportTrackerMigration(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	return writeJSON(out, packet)
}
