package state

import (
	"context"
	"fmt"

	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/rehearsal"
)

// VNextRehearsalResult retains both the canonical legacy handoff and the
// deterministic receipt from its staged vNext import.
type VNextRehearsalResult struct {
	Archive []byte
	Report  rehearsal.Report
}

// RunVNextRehearsal exports one verified legacy backup and imports the
// canonical archive into a caller-owned isolated vNext store. It has no
// activation, cutover, tracker, or live-database mutation surface.
func RunVNextRehearsal(
	ctx context.Context,
	options VNextRehearsalExportOptions,
	destination *continuitysqlite.Store,
) (VNextRehearsalResult, error) {
	result := VNextRehearsalResult{}
	if destination == nil {
		return result, fmt.Errorf("run vNext rehearsal: destination store is nil")
	}
	encoded, err := ExportVNextRehearsalArchive(ctx, options)
	if err != nil {
		return result, fmt.Errorf("run vNext rehearsal: %w", err)
	}
	result.Archive = encoded
	report, err := rehearsal.Import(ctx, encoded, destination)
	if err != nil {
		return result, fmt.Errorf("run vNext rehearsal: %w", err)
	}
	result.Report = report
	return result, nil
}
