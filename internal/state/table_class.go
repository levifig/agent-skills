package state

import (
	"fmt"
	"sort"
)

// TableClass is the sync disposition for one project-scoped table. Every table
// that carries project_id must declare exactly one class. Gone tables are
// recorded so completeness tests stay closed when schema drops them.
type TableClass string

const (
	TableClassSync         TableClass = "sync"
	TableClassLocalArchive TableClass = "local-archive"
	TableClassMachineLocal TableClass = "machine-local"
	TableClassGone         TableClass = "gone"
)

// fossilEntityKinds are vocabulary fossils whose relationship edges are pruned
// during migration and never participate in sync.
var fossilEntityKinds = []string{
	"plan",
	"spec",
	"task",
	"intent",
	"brainstorm",
}

// goneEntityKinds are deleted schema kinds whose polymorphic references were
// removed by migrations 0018 and 0021.
var goneEntityKinds = []string{
	"finding",
	"verdict",
	"run",
	"report",
	"council",
	"shaping_draft",
}

// projectScopedTableClasses is the closed inventory lock for LOAF-81. Keys are
// every project-scoped table that exists or was intentionally removed.
var projectScopedTableClasses = map[string]TableClass{
	// Sync — minimal core (LOAF-62): identity evidence, refs/contracts, ledger,
	// releases. FTS and machine-local projections are excluded.
	"project_paths":                  TableClassSync,
	"project_conf_labels":            TableClassMachineLocal,
	"project_attachment_evidence":    TableClassSync,
	"aliases":                        TableClassSync,
	"sources":                        TableClassSync,
	"ideas":                          TableClassSync,
	"sparks":                         TableClassSync,
	"journal_entries":                TableClassSync,
	"journal_origins":                TableClassSync,
	"journal_deferrals":              TableClassSync,
	"handoffs":                       TableClassSync,
	"facts":                          TableClassSync,
	"fact_env_clocks":                TableClassSync,
	"relationships":                  TableClassSync,
	"tags":                           TableClassSync,
	"entity_tags":                    TableClassSync,
	"bundles":                        TableClassSync,
	"bundle_members":                 TableClassSync,
	"backend_mappings":               TableClassSync,
	"issues":                         TableClassSync,
	"issue_criteria":                 TableClassSync,
	"issue_criterion_claims":         TableClassSync,
	"issue_identity":                 TableClassSync,
	"work_contracts":                 TableClassSync,
	"work_contract_criteria":         TableClassSync,
	"work_contract_criterion_claims": TableClassSync,
	"work_contract_workspace":        TableClassSync,
	"work_contract_mappings":         TableClassSync,
	"work_contract_receipts":         TableClassSync,
	"releases":                       TableClassSync,
	"release_members":                TableClassSync,

	// Local archive — vocabulary fossils stay readable locally and never sync.
	"specs":                        TableClassLocalArchive,
	"tasks":                        TableClassLocalArchive,
	"plans":                        TableClassLocalArchive,
	"events":                       TableClassLocalArchive,
	"brainstorms":                  TableClassLocalArchive,
	"intents":                      TableClassLocalArchive,
	"intent_snapshots":             TableClassLocalArchive,
	"intent_deferrals":             TableClassLocalArchive,
	"intent_dispositions":          TableClassLocalArchive,
	"intent_operations":            TableClassLocalArchive,
	"explorations":                 TableClassLocalArchive,
	"exploration_checkpoints":      TableClassLocalArchive,
	"exploration_checkpoint_items": TableClassLocalArchive,

	// Machine-local — hook trust, conversation handles, observations, session
	// snapshots, and locally rebuilt FTS source tables never sync.
	"sessions":                         TableClassMachineLocal,
	"session_state_snapshots":          TableClassMachineLocal,
	"hook_events":                      TableClassMachineLocal,
	"exports":                          TableClassMachineLocal,
	"artifact_bodies":                  TableClassMachineLocal,
	"docs_index":                       TableClassMachineLocal,
	"logical_conversations":            TableClassMachineLocal,
	"conversation_handles":             TableClassMachineLocal,
	"conversation_log_refs":            TableClassMachineLocal,
	"exploration_conversations":        TableClassMachineLocal,
	"journal_conversation_handles":     TableClassMachineLocal,
	"source_availability_observations": TableClassMachineLocal,

	// Client sync engine machine-local state (migration 0023).
	"sync_outbound_queue":       TableClassMachineLocal,
	"sync_project_cursors":      TableClassMachineLocal,
	"fact_replay_discrepancies": TableClassMachineLocal,

	// Gone — schema deleted; tests refuse reintroduction (LOAF-79/80).
	"findings":       TableClassGone,
	"verdicts":       TableClassGone,
	"runs":           TableClassGone,
	"reports":        TableClassGone,
	"councils":       TableClassGone,
	"shaping_drafts": TableClassGone,
}

// ProjectScopedTableClass returns the declared class for one project-scoped table.
func ProjectScopedTableClass(table string) (TableClass, bool) {
	class, ok := projectScopedTableClasses[table]
	return class, ok
}

// ProjectScopedTableClasses returns a sorted copy of the inventory registry.
func ProjectScopedTableClasses() map[string]TableClass {
	out := make(map[string]TableClass, len(projectScopedTableClasses))
	for table, class := range projectScopedTableClasses {
		out[table] = class
	}
	return out
}

// ProjectScopedSyncTables returns project-scoped tables that participate in
// storage-home merge and future sync transport.
func ProjectScopedSyncTables() []string {
	return tablesForClass(TableClassSync)
}

// FossilEntityKinds returns vocabulary fossil entity kinds whose relationship
// edges are pruned and never synced.
func FossilEntityKinds() []string {
	return append([]string(nil), fossilEntityKinds...)
}

// GoneEntityKinds returns deleted entity kinds whose polymorphic references
// must stay absent.
func GoneEntityKinds() []string {
	return append([]string(nil), goneEntityKinds...)
}

// ProjectScopedLiveTables returns every project-scoped table that still exists
// in the live schema (sync, local-archive, and machine-local).
func ProjectScopedLiveTables() []string {
	var tables []string
	for table, class := range projectScopedTableClasses {
		if class == TableClassGone {
			continue
		}
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

// PrunableNeighborEntityKinds returns deleted entity kinds whose polymorphic
// neighbor edges should be skipped during trace/link hydration.
func PrunableNeighborEntityKinds() []string {
	return append([]string(nil), goneEntityKinds...)
}

func tablesForClass(class TableClass) []string {
	var tables []string
	for table, candidate := range projectScopedTableClasses {
		if candidate == class {
			tables = append(tables, table)
		}
	}
	sort.Strings(tables)
	return tables
}

func validateProjectScopedTableInventory() error {
	for table, class := range projectScopedTableClasses {
		switch class {
		case TableClassSync, TableClassLocalArchive, TableClassMachineLocal, TableClassGone:
		default:
			return fmt.Errorf("table %s has unknown class %q", table, class)
		}
	}
	return nil
}
