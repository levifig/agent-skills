package state

import (
	"context"
	"database/sql"
	"fmt"
)

// AliasParityDivergenceCode is the stable diagnostic code when raw entity
// counts diverge from alias-reachable counts, or dangling aliases exist.
const AliasParityDivergenceCode = "alias-parity-diverged"

// AliasParityMultiAliasCode is the stable diagnostic code when an entity holds
// more than one alias. Multi-alias is countable and warning-only; it does not
// gate Ready and names no repair command.
const AliasParityMultiAliasCode = "alias-multi-alias"

// AliasParityClearCode is the info-severity receipt when every project/table is at parity.
const AliasParityClearCode = "alias-parity-clear"

// AliasParityRepairCommand is the preview-form migration that repairs alias orphans.
const AliasParityRepairCommand = "loaf state migrate alias-orphans"

// AliasParityTable holds raw vs alias-reachable counts for one project entity
// table. AliasReachableCount mirrors what the list surfaces return: they INNER
// JOIN through aliases, so their cardinality is the number of alias rows that
// resolve, not the number of entities that happen to hold one. AliasedEntities
// counts the distinct entities behind those rows, so both directions of
// divergence — entities with no alias, entities with more than one — are
// visible instead of cancelling out.
type AliasParityTable struct {
	ProjectID           string `json:"project_id"`
	Kind                string `json:"kind"`
	Table               string `json:"table"`
	Namespace           string `json:"namespace"`
	RawCount            int    `json:"raw_count"`
	AliasReachableCount int    `json:"alias_reachable_count"`
	AliasedEntities     int    `json:"aliased_entities"`
	OrphanDelta         int    `json:"orphan_delta"`
	MultiAlias          int    `json:"multi_alias"`
	DanglingAliases     int    `json:"dangling_aliases"`
	// Inconsistent is true when the table's counts fail the internal identity
	// check (orphan_delta != raw_count - aliased_entities). Counted as
	// divergence rather than an inspect error so Mode stays ready.
	Inconsistent  bool   `json:"inconsistent,omitempty"`
	Inconsistency string `json:"inconsistency,omitempty"`
}

// AliasParity is the read-only doctor report for entity/alias identity parity.
type AliasParity struct {
	Tables              []AliasParityTable `json:"tables"`
	ProjectsChecked     int                `json:"projects_checked"`
	TablesChecked       int                `json:"tables_checked"`
	RawCount            int                `json:"raw_count"`
	AliasReachableCount int                `json:"alias_reachable_count"`
	AliasedEntities     int                `json:"aliased_entities"`
	OrphanDelta         int                `json:"orphan_delta"`
	MultiAlias          int                `json:"multi_alias"`
	DanglingAliases     int                `json:"dangling_aliases"`
	Ready               bool               `json:"ready"`
	Inconsistencies     []string           `json:"inconsistencies,omitempty"`
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
			parity.AliasedEntities += row.AliasedEntities
			parity.OrphanDelta += row.OrphanDelta
			parity.MultiAlias += row.MultiAlias
			parity.DanglingAliases += row.DanglingAliases
			if row.Inconsistent && row.Inconsistency != "" {
				parity.Inconsistencies = append(parity.Inconsistencies, row.Inconsistency)
			}
		}
	}
	parity.TablesChecked = len(parity.Tables)
	// Multi-alias is warning-only and does not gate Ready; only orphan/dangling
	// damage and internal count inconsistency do.
	if parity.OrphanDelta > 0 || parity.DanglingAliases > 0 || len(parity.Inconsistencies) > 0 {
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

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("begin alias parity snapshot for %s: %w", table.table, err)
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx, fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE project_id = ?`, quotedTable,
	), projectID).Scan(&result.RawCount); err != nil {
		return result, fmt.Errorf("count raw %s rows: %w", table.table, err)
	}

	// One row per alias — the exact cardinality `loaf <kind> list` returns.
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM aliases AS a
JOIN %s AS e ON e.project_id = a.project_id AND e.id = a.entity_id
WHERE a.project_id = ? AND a.entity_kind = ? AND a.namespace = ?
`, quotedTable), projectID, table.kind, table.namespace).Scan(&result.AliasReachableCount); err != nil {
		return result, fmt.Errorf("count alias-reachable %s rows: %w", table.table, err)
	}

	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
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
`, quotedTable), projectID, table.kind, table.namespace).Scan(&result.AliasedEntities); err != nil {
		return result, fmt.Errorf("count aliased %s entities: %w", table.table, err)
	}

	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
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

	// Dead aliases only — a forward reference the importer registered for an
	// artifact that has no row yet is not divergence. See
	// aliasOrphanDeadAliasPredicate: detector and repair share one definition.
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM aliases AS a
WHERE a.project_id = ?
  AND a.entity_kind = ?
  AND a.namespace = ?`+aliasOrphanDeadAliasPredicate, quotedTable), projectID, table.kind, table.namespace).Scan(&result.DanglingAliases); err != nil {
		return result, fmt.Errorf("count dangling %s aliases: %w", table.table, err)
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit alias parity snapshot for %s: %w", table.table, err)
	}

	result.MultiAlias = result.AliasReachableCount - result.AliasedEntities
	if result.OrphanDelta != result.RawCount-result.AliasedEntities {
		result.Inconsistent = true
		result.Inconsistency = fmt.Sprintf(
			"alias parity internal inconsistency for %s project %s: orphan_delta=%d raw=%d aliased=%d",
			table.table, projectID, result.OrphanDelta, result.RawCount, result.AliasedEntities,
		)
	}
	return result, nil
}

func aliasParityDiagnostics(parity AliasParity) []Diagnostic {
	var out []Diagnostic
	if !parity.Ready {
		out = append(out, aliasParityDivergenceDiagnostic(parity))
	} else if parity.MultiAlias == 0 {
		out = append(out, aliasParityClearDiagnostic(parity))
	}
	if parity.MultiAlias > 0 {
		out = append(out, aliasParityMultiAliasDiagnostic(parity))
	}
	return out
}

func aliasParityDivergenceDiagnostic(parity AliasParity) Diagnostic {
	divergent := make([]map[string]any, 0)
	for _, table := range parity.Tables {
		if table.OrphanDelta == 0 && table.DanglingAliases == 0 && !table.Inconsistent {
			continue
		}
		row := map[string]any{
			"project_id":            table.ProjectID,
			"kind":                  table.Kind,
			"table":                 table.Table,
			"namespace":             table.Namespace,
			"raw_count":             table.RawCount,
			"alias_reachable_count": table.AliasReachableCount,
			"aliased_entities":      table.AliasedEntities,
			"orphan_delta":          table.OrphanDelta,
			"multi_alias":           table.MultiAlias,
			"dangling_aliases":      table.DanglingAliases,
		}
		if table.Inconsistent {
			row["inconsistent"] = true
			row["inconsistency"] = table.Inconsistency
		}
		divergent = append(divergent, row)
	}
	message := fmt.Sprintf(
		"alias parity diverged (orphan_delta=%d, multi_alias=%d, dangling_aliases=%d); run: %s",
		parity.OrphanDelta,
		parity.MultiAlias,
		parity.DanglingAliases,
		AliasParityRepairCommand,
	)
	if len(parity.Inconsistencies) > 0 {
		message = fmt.Sprintf(
			"alias parity diverged (orphan_delta=%d, multi_alias=%d, dangling_aliases=%d, inconsistencies=%d); run: %s",
			parity.OrphanDelta,
			parity.MultiAlias,
			parity.DanglingAliases,
			len(parity.Inconsistencies),
			AliasParityRepairCommand,
		)
	}
	details := map[string]any{
		"raw_count":             parity.RawCount,
		"alias_reachable_count": parity.AliasReachableCount,
		"aliased_entities":      parity.AliasedEntities,
		"orphan_delta":          parity.OrphanDelta,
		"multi_alias":           parity.MultiAlias,
		"dangling_aliases":      parity.DanglingAliases,
		"tables":                divergent,
		"preview_command":       AliasParityRepairCommand,
	}
	if len(parity.Inconsistencies) > 0 {
		details["inconsistencies"] = parity.Inconsistencies
	}
	return Diagnostic{
		Severity: "error",
		Code:     AliasParityDivergenceCode,
		Category: RepairCategoryAliasIdentity,
		Policy:   DiagnosticPolicyInvalidLocalData,
		Message:  message,
		Details:  details,
	}
}

func aliasParityMultiAliasDiagnostic(parity AliasParity) Diagnostic {
	divergent := make([]map[string]any, 0)
	for _, table := range parity.Tables {
		if table.MultiAlias == 0 {
			continue
		}
		divergent = append(divergent, map[string]any{
			"project_id":            table.ProjectID,
			"kind":                  table.Kind,
			"table":                 table.Table,
			"namespace":             table.Namespace,
			"raw_count":             table.RawCount,
			"alias_reachable_count": table.AliasReachableCount,
			"aliased_entities":      table.AliasedEntities,
			"orphan_delta":          table.OrphanDelta,
			"multi_alias":           table.MultiAlias,
			"dangling_aliases":      table.DanglingAliases,
		})
	}
	return Diagnostic{
		Severity: "warn",
		Code:     AliasParityMultiAliasCode,
		Category: RepairCategoryAliasIdentity,
		Message: fmt.Sprintf(
			"alias multi-alias present (multi_alias=%d); entities hold more than one alias; no automated repair",
			parity.MultiAlias,
		),
		Details: map[string]any{
			"raw_count":             parity.RawCount,
			"alias_reachable_count": parity.AliasReachableCount,
			"aliased_entities":      parity.AliasedEntities,
			"orphan_delta":          parity.OrphanDelta,
			"multi_alias":           parity.MultiAlias,
			"dangling_aliases":      parity.DanglingAliases,
			"tables":                divergent,
		},
	}
}

func aliasParityClearDiagnostic(parity AliasParity) Diagnostic {
	return Diagnostic{
		Severity: "info",
		Code:     AliasParityClearCode,
		Category: RepairCategoryAliasIdentity,
		Message: fmt.Sprintf(
			"alias parity clear: %d project(s), %d table check(s); raw_count=%d equals alias_reachable_count; multi_alias=0; dangling_aliases=0",
			parity.ProjectsChecked,
			parity.TablesChecked,
			parity.RawCount,
		),
		Details: map[string]any{
			"projects_checked":      parity.ProjectsChecked,
			"tables_checked":        parity.TablesChecked,
			"raw_count":             parity.RawCount,
			"alias_reachable_count": parity.AliasReachableCount,
			"aliased_entities":      parity.AliasedEntities,
			"orphan_delta":          parity.OrphanDelta,
			"multi_alias":           parity.MultiAlias,
			"dangling_aliases":      parity.DanglingAliases,
		},
	}
}
