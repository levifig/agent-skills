package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestStateDoctorAliasParityCleanFixture(t *testing.T) {
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	resolver := PathResolver{StateHome: stateHome}

	seedTask(t, stateHome, root, projectID, "task:clean0000000000000001", "Clean Task", "todo", "2026-06-24T13:03:00Z", true, "TASK-CLEAN")
	seedSpec(t, stateHome, root, projectID, "spec:clean0000000000000001", "Clean Spec", "active", "2026-06-24T13:03:00Z", true, "SPEC-CLEAN")

	status, err := InspectWithOptions(root, resolver, InspectOptions{AliasParity: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q; diagnostics = %#v", status.Mode, ModeSQLiteReady, status.Diagnostics)
	}
	assertNoDiagnostic(t, status.Diagnostics, AliasParityDivergenceCode)

	diagnostic := findDiagnostic(t, status.Diagnostics, AliasParityClearCode)
	if diagnostic.Severity != "info" || diagnostic.Category != RepairCategoryAliasIdentity {
		t.Fatalf("alias parity clear diagnostic = %#v, want info/%s", diagnostic, RepairCategoryAliasIdentity)
	}
	wantTablesChecked := len(aliasOrphanEntityTables)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "projects_checked", 1)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "tables_checked", wantTablesChecked)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "raw_count", 2)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "alias_reachable_count", 2)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "orphan_delta", 0)
	assertDiagnosticDetail(t, status.Diagnostics, AliasParityClearCode, "dangling_aliases", 0)
	if !strings.Contains(diagnostic.Message, "alias parity clear") {
		t.Fatalf("clear message %q missing summary prefix", diagnostic.Message)
	}
	if !strings.Contains(diagnostic.Message, "dangling_aliases=0") {
		t.Fatalf("clear message %q missing dangling_aliases=0", diagnostic.Message)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	parity, err := InspectAliasParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectAliasParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("parity = %#v, want Ready=true", parity)
	}
	if parity.OrphanDelta != 0 || parity.DanglingAliases != 0 {
		t.Fatalf("parity deltas = orphan=%d dangling=%d, want 0/0", parity.OrphanDelta, parity.DanglingAliases)
	}
	taskRow := findAliasParityTable(t, parity, projectID, "tasks")
	if taskRow.RawCount != 1 || taskRow.AliasReachableCount != 1 || taskRow.OrphanDelta != 0 {
		t.Fatalf("tasks parity = %#v, want raw=1 reachable=1 orphan=0", taskRow)
	}
}

func TestAliasParityStaysOffTheDefaultInspectPath(t *testing.T) {
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	resolver := PathResolver{StateHome: stateHome}
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-HOTPATH"
	seedTask(t, stateHome, root, projectID, stableMigrationID("task", projectID, alias), "Hot Path Task", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, stableMigrationID("task", legacyID, alias), "Hot Path Task", "todo", "2026-06-13T10:00:00Z", false, "")

	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q; diagnostics = %#v", status.Mode, ModeSQLiteReady, status.Diagnostics)
	}
	// Every list/read command calls Inspect; a global 27-project table scan does
	// not belong on that path.
	assertNoDiagnostic(t, status.Diagnostics, AliasParityDivergenceCode)
	assertNoDiagnostic(t, status.Diagnostics, AliasParityClearCode)
}

func TestAliasParityCountsAliasRowsNotAliasedEntities(t *testing.T) {
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	seedTask(t, stateHome, root, projectID, "task:multialias00000000001", "Two Aliases", "todo", "2026-06-24T13:03:00Z", true, "TASK-ONE")
	mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'task', 'task:multialias00000000001', 'task', 'TASK-TWO', ?, ?)
`, "alias:multialias0000000001", projectID, "2026-06-24T13:03:00Z", "2026-06-24T13:03:00Z")

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	parity, err := InspectAliasParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectAliasParity() error = %v", err)
	}
	tasks := findAliasParityTable(t, parity, projectID, "tasks")
	// `loaf task list` INNER JOINs aliases, so it returns two rows for one task.
	if tasks.AliasReachableCount != 2 || tasks.RawCount != 1 || tasks.MultiAlias != 1 {
		t.Fatalf("tasks parity = %#v, want raw=1 reachable=2 multi_alias=1", tasks)
	}
	if parity.Ready {
		t.Fatalf("parity = %#v, want Ready=false while the scanner and list disagree", parity)
	}
}

func TestStateDoctorAliasParityOrphanFinding(t *testing.T) {
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	resolver := PathResolver{StateHome: stateHome}
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-ORPHAN"
	twinID := stableMigrationID("task", projectID, alias)
	orphanID := stableMigrationID("task", legacyID, alias)

	seedTask(t, stateHome, root, projectID, twinID, "Twin Task", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanID, "Twin Task", "todo", "2026-06-13T10:00:00Z", false, "")

	status, err := InspectWithOptions(root, resolver, InspectOptions{AliasParity: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want usable sqlite-ready despite alias damage; diagnostics = %#v", status.Mode, status.Diagnostics)
	}

	assertNoDiagnostic(t, status.Diagnostics, AliasParityClearCode)
	diagnostic := findDiagnostic(t, status.Diagnostics, AliasParityDivergenceCode)
	if diagnostic.Severity != "error" || diagnostic.Category != RepairCategoryAliasIdentity || diagnostic.Policy != DiagnosticPolicyInvalidLocalData {
		t.Fatalf("alias parity diagnostic = %#v, want error/%s/%s", diagnostic, RepairCategoryAliasIdentity, DiagnosticPolicyInvalidLocalData)
	}
	if diagnostic.Details["orphan_delta"] != 1 {
		t.Fatalf("orphan_delta = %#v, want 1", diagnostic.Details["orphan_delta"])
	}
	if diagnostic.Details["dangling_aliases"] != 0 {
		t.Fatalf("dangling_aliases = %#v, want 0", diagnostic.Details["dangling_aliases"])
	}
	if diagnostic.Details["raw_count"] != 2 {
		t.Fatalf("raw_count = %#v, want 2", diagnostic.Details["raw_count"])
	}
	if diagnostic.Details["alias_reachable_count"] != 1 {
		t.Fatalf("alias_reachable_count = %#v, want 1", diagnostic.Details["alias_reachable_count"])
	}
	if !strings.Contains(diagnostic.Message, AliasParityRepairCommand) {
		t.Fatalf("message %q missing repair command %q", diagnostic.Message, AliasParityRepairCommand)
	}
	if !strings.Contains(diagnostic.Message, "orphan_delta=1") {
		t.Fatalf("message %q missing orphan_delta=1", diagnostic.Message)
	}

	table := findAliasParityDetailTable(t, diagnostic, projectID, "tasks")
	if table["raw_count"] != 2 || table["alias_reachable_count"] != 1 || table["orphan_delta"] != 1 || table["dangling_aliases"] != 0 {
		t.Fatalf("tasks detail = %#v, want raw=2 reachable=1 orphan=1 dangling=0", table)
	}

	action := findRepairAction(t, RepairPlanForStatus(Status{DatabasePath: status.DatabasePath, Diagnostics: status.Diagnostics}), "migrate-alias-orphans")
	if action.DiagnosticCode != AliasParityDivergenceCode || action.Category != RepairCategoryAliasIdentity || action.Safe || action.Command != AliasParityRepairCommand {
		t.Fatalf("repair action = %#v, want migrate-alias-orphans unsafe preview command", action)
	}
}

func TestStateDoctorAliasParityDanglingAliasFinding(t *testing.T) {
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	resolver := PathResolver{StateHome: stateHome}

	seedTask(t, stateHome, root, projectID, "task:kept000000000000000001", "Kept Task", "todo", "2026-06-24T13:03:00Z", true, "TASK-KEPT")
	danglingAliasID := "alias:dangling000000000001"
	mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'task', 'task:missing0000000000001', 'task', 'TASK-MISSING', ?, ?)
`, danglingAliasID, projectID, "2026-06-24T13:03:00Z", "2026-06-24T13:03:00Z")

	status, err := InspectWithOptions(root, resolver, InspectOptions{AliasParity: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want usable sqlite-ready; diagnostics = %#v", status.Mode, status.Diagnostics)
	}

	assertNoDiagnostic(t, status.Diagnostics, AliasParityClearCode)
	diagnostic := findDiagnostic(t, status.Diagnostics, AliasParityDivergenceCode)
	if diagnostic.Details["dangling_aliases"] != 1 {
		t.Fatalf("dangling_aliases = %#v, want 1", diagnostic.Details["dangling_aliases"])
	}
	if diagnostic.Details["orphan_delta"] != 0 {
		t.Fatalf("orphan_delta = %#v, want 0", diagnostic.Details["orphan_delta"])
	}
	if !strings.Contains(diagnostic.Message, "dangling_aliases=1") {
		t.Fatalf("message %q missing dangling_aliases=1", diagnostic.Message)
	}
	if !strings.Contains(diagnostic.Message, AliasParityRepairCommand) {
		t.Fatalf("message %q missing repair command %q", diagnostic.Message, AliasParityRepairCommand)
	}

	table := findAliasParityDetailTable(t, diagnostic, projectID, "tasks")
	if table["raw_count"] != 1 || table["alias_reachable_count"] != 1 || table["orphan_delta"] != 0 || table["dangling_aliases"] != 1 {
		t.Fatalf("tasks detail = %#v, want raw=1 reachable=1 orphan=0 dangling=1", table)
	}

	action := findRepairAction(t, RepairPlanForStatus(Status{DatabasePath: status.DatabasePath, Diagnostics: status.Diagnostics}), "migrate-alias-orphans")
	if action.Command != AliasParityRepairCommand {
		t.Fatalf("repair command = %q, want %q", action.Command, AliasParityRepairCommand)
	}
}

func TestStateDoctorAliasParityDiagnosticPerformsNoWrites(t *testing.T) {
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	resolver := PathResolver{StateHome: stateHome}
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-NOWRITE"
	twinID := stableMigrationID("task", projectID, alias)
	orphanID := stableMigrationID("task", legacyID, alias)

	seedTask(t, stateHome, root, projectID, twinID, "No Write Twin", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanID, "No Write Twin", "todo", "2026-06-13T10:00:00Z", false, "")
	mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'task', 'task:missing-nowrite000001', 'task', 'TASK-DANGLING-NW', ?, ?)
`, "alias:dangling-nowrite000001", projectID, "2026-06-24T13:03:00Z", "2026-06-24T13:03:00Z")

	dbPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	// Checkpoint so file bytes are stable under WAL.
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		store.Close()
		t.Fatalf("wal_checkpoint: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	removeSQLiteSidecars(t, dbPath)

	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	beforeHash := sha256.Sum256(before)

	status, err := InspectWithOptions(root, resolver, InspectOptions{AliasParity: true})
	if err != nil {
		t.Fatalf("InspectWithOptions() error = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, AliasParityDivergenceCode)

	// Ensure any read-only connection is fully closed before re-hashing.
	removeSQLiteSidecars(t, dbPath)
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	afterHash := sha256.Sum256(after)
	if !bytes.Equal(beforeHash[:], afterHash[:]) {
		t.Fatalf("InspectWithOptions mutated database bytes: before=%x after=%x", beforeHash, afterHash)
	}
	if !entityExists(t, stateHome, root, "tasks", orphanID) {
		t.Fatal("orphan row missing after InspectWithOptions")
	}
	if !entityExists(t, stateHome, root, "aliases", "alias:dangling-nowrite000001") {
		t.Fatal("dangling alias missing after InspectWithOptions")
	}
}

func findAliasParityTable(t *testing.T, parity AliasParity, projectID, table string) AliasParityTable {
	t.Helper()
	for _, row := range parity.Tables {
		if row.ProjectID == projectID && row.Table == table {
			return row
		}
	}
	t.Fatalf("parity table %s for project %s not found in %#v", table, projectID, parity.Tables)
	return AliasParityTable{}
}

func findAliasParityDetailTable(t *testing.T, diagnostic Diagnostic, projectID, table string) map[string]any {
	t.Helper()
	raw, ok := diagnostic.Details["tables"]
	if !ok {
		t.Fatalf("diagnostic details missing tables: %#v", diagnostic.Details)
	}
	switch tables := raw.(type) {
	case []map[string]any:
		for _, row := range tables {
			if row["project_id"] == projectID && row["table"] == table {
				return row
			}
		}
	case []any:
		for _, item := range tables {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if row["project_id"] == projectID && row["table"] == table {
				return row
			}
		}
	default:
		t.Fatalf("tables detail type %T = %#v", raw, raw)
	}
	t.Fatalf("table %s for project %s not found in %#v", table, projectID, raw)
	return nil
}
