package state

import (
	"context"
	"fmt"
)

// AliasParityDivergenceCode is the stable diagnostic code when raw entity
// counts diverge from alias-reachable counts, or dangling aliases exist.
const AliasParityDivergenceCode = "alias-parity-diverged"

// AliasParityClearCode is the info-severity receipt when every project/table is at parity.
const AliasParityClearCode = "alias-parity-clear"

// AliasParityRepairCommand is the preview-form migration that repairs alias orphans.
const AliasParityRepairCommand = "loaf state migrate alias-orphans"

// AliasParityTable holds raw vs alias-reachable counts for one project entity table.
type AliasParityTable struct {
	ProjectID           string `json:"project_id"`
	Kind                string `json:"kind"`
	Table               string `json:"table"`
	Namespace           string `json:"namespace"`
	RawCount            int    `json:"raw_count"`
	AliasReachableCount int    `json:"alias_reachable_count"`
	OrphanDelta         int    `json:"orphan_delta"`
	DanglingAliases     int    `json:"dangling_aliases"`
}

// AliasParity is the read-only doctor report for entity/alias identity parity.
type AliasParity struct {
	Tables              []AliasParityTable `json:"tables"`
	ProjectsChecked     int                `json:"projects_checked"`
	TablesChecked       int                `json:"tables_checked"`
	RawCount            int                `json:"raw_count"`
	AliasReachableCount int                `json:"alias_reachable_count"`
	OrphanDelta         int                `json:"orphan_delta"`
	DanglingAliases     int                `json:"dangling_aliases"`
	Ready               bool               `json:"ready"`
}

// InspectAliasParity compares raw entity row counts to alias-joined counts and
// counts dangling aliases for every project and each entity table. Read-only.
func InspectAliasParity(ctx context.Context, store *Store) (AliasParity, error) {
	if store == nil || store.db == nil {
		return AliasParity{}, fmt.Errorf("inspect alias parity: store is nil")
	}

	projectIDs, err := listAliasParityProjectIDs(ctx, store)
	if err != nil {
		return AliasParity{}, err
	}

	parity := AliasParity{
		Tables:          []AliasParityTable{},
		ProjectsChecked: len(projectIDs),
		Ready:           true,
	}
	for _, projectID := range projectIDs {
		for _, table := range aliasOrphanEntityTables {
			row, err := inspectAliasParityTable(ctx, store, projectID, table)
			if err != nil {
				return AliasParity{}, err
			}
			parity.Tables = append(parity.Tables, row)
			parity.RawCount += row.RawCount
			parity.AliasReachableCount += row.AliasReachableCount
			parity.OrphanDelta += row.OrphanDelta
			parity.DanglingAliases += row.DanglingAliases
		}
	}
	parity.TablesChecked = len(parity.Tables)
	if parity.OrphanDelta > 0 || parity.DanglingAliases > 0 {
		parity.Ready = false
	}
	return parity, nil
}

func listAliasParityProjectIDs(ctx context.Context, store *Store) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id FROM projects ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list projects for alias parity: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan project id for alias parity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects for alias parity: %w", err)
	}
	return ids, nil
}

func inspectAliasParityTable(ctx context.Context, store *Store, projectID string, table aliasOrphanEntityTable) (AliasParityTable, error) {
	result := AliasParityTable{
		ProjectID: projectID,
		Kind:      table.kind,
		Table:     table.table,
		Namespace: table.namespace,
	}
	quotedTable := quoteSQLiteIdentifier(table.table)

	if err := store.db.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE project_id = ?`, quotedTable,
	), projectID).Scan(&result.RawCount); err != nil {
		return result, fmt.Errorf("count raw %s rows: %w", table.table, err)
	}

	if err := store.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s AS e
WHERE e.project_id = ?
  AND EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
`, quotedTable), projectID, table.kind, table.namespace).Scan(&result.AliasReachableCount); err != nil {
		return result, fmt.Errorf("count alias-reachable %s rows: %w", table.table, err)
	}

	if err := store.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s AS e
WHERE e.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
`, quotedTable), projectID, table.kind, table.namespace).Scan(&result.OrphanDelta); err != nil {
		return result, fmt.Errorf("count orphan %s rows: %w", table.table, err)
	}

	if err := store.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM aliases AS a
WHERE a.project_id = ?
  AND a.entity_kind = ?
  AND a.namespace = ?
  AND NOT EXISTS (
    SELECT 1 FROM %s AS e
    WHERE e.project_id = a.project_id AND e.id = a.entity_id
  )
`, quotedTable), projectID, table.kind, table.namespace).Scan(&result.DanglingAliases); err != nil {
		return result, fmt.Errorf("count dangling %s aliases: %w", table.table, err)
	}

	if result.OrphanDelta != result.RawCount-result.AliasReachableCount {
		return result, fmt.Errorf(
			"alias parity internal inconsistency for %s project %s: orphan_delta=%d raw=%d reachable=%d",
			table.table, projectID, result.OrphanDelta, result.RawCount, result.AliasReachableCount,
		)
	}
	return result, nil
}

func aliasParityDiagnostic(parity AliasParity) Diagnostic {
	if parity.Ready {
		return aliasParityClearDiagnostic(parity)
	}
	divergent := make([]map[string]any, 0)
	for _, table := range parity.Tables {
		if table.OrphanDelta == 0 && table.DanglingAliases == 0 {
			continue
		}
		divergent = append(divergent, map[string]any{
			"project_id":            table.ProjectID,
			"kind":                  table.Kind,
			"table":                 table.Table,
			"namespace":             table.Namespace,
			"raw_count":             table.RawCount,
			"alias_reachable_count": table.AliasReachableCount,
			"orphan_delta":          table.OrphanDelta,
			"dangling_aliases":      table.DanglingAliases,
		})
	}
	return Diagnostic{
		Severity: "error",
		Code:     AliasParityDivergenceCode,
		Category: RepairCategoryAliasIdentity,
		Policy:   DiagnosticPolicyInvalidLocalData,
		Message: fmt.Sprintf(
			"alias parity diverged (orphan_delta=%d, dangling_aliases=%d); run: %s",
			parity.OrphanDelta,
			parity.DanglingAliases,
			AliasParityRepairCommand,
		),
		Details: map[string]any{
			"raw_count":             parity.RawCount,
			"alias_reachable_count": parity.AliasReachableCount,
			"orphan_delta":          parity.OrphanDelta,
			"dangling_aliases":      parity.DanglingAliases,
			"tables":                divergent,
			"preview_command":       AliasParityRepairCommand,
		},
	}
}

func aliasParityClearDiagnostic(parity AliasParity) Diagnostic {
	return Diagnostic{
		Severity: "info",
		Code:     AliasParityClearCode,
		Category: RepairCategoryAliasIdentity,
		Message: fmt.Sprintf(
			"alias parity clear: %d project(s), %d table check(s); raw_count=%d equals alias_reachable_count; dangling_aliases=0",
			parity.ProjectsChecked,
			parity.TablesChecked,
			parity.RawCount,
		),
		Details: map[string]any{
			"projects_checked":      parity.ProjectsChecked,
			"tables_checked":        parity.TablesChecked,
			"raw_count":             parity.RawCount,
			"alias_reachable_count": parity.AliasReachableCount,
			"orphan_delta":          parity.OrphanDelta,
			"dangling_aliases":      parity.DanglingAliases,
		},
	}
}
