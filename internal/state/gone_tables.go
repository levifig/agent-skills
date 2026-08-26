package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// goneTables are project-scoped schema tables that were deleted and must not
// reappear. Migration 0018 drops the zero-row findings/verdicts/runs substrate;
// ValidateCurrentSchema refuses a database that still carries them.
var goneTables = []string{
	"findings",
	"verdicts",
	"runs",
}

// GoneTables returns the closed set of tables that must stay absent.
func GoneTables() []string {
	return append([]string(nil), goneTables...)
}

func validateGoneTablesAbsent(ctx context.Context, db *sql.DB) error {
	var present []string
	for _, table := range goneTables {
		var name string
		err := db.QueryRowContext(ctx, `
SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?
`, table).Scan(&name)
		switch {
		case err == nil:
			present = append(present, table)
		case err == sql.ErrNoRows:
			continue
		default:
			return fmt.Errorf("inspect gone table %s: %w", table, err)
		}
	}
	if len(present) == 0 {
		return nil
	}
	sort.Strings(present)
	return fmt.Errorf("gone tables must not exist: %s", strings.Join(present, ", "))
}
