package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// entityDescriptor registers one SQLite-backed entity kind. Every surface that
// enumerates kinds — alias resolution, internal-ID resolution, trace details,
// link validation, export, deletion, mapping audits, and test fixtures —
// consults this registry instead of a local switch, so adding a kind is one
// explicit registration rather than a sweep of distributed lists.
type entityDescriptor struct {
	// Kind is the canonical entity-kind string stored in aliases,
	// relationships, events, and entity_tags rows.
	Kind string
	// Table is the backing SQLite table.
	Table string
	// InternalIDResolvable marks kinds whose rows may be resolved directly by
	// internal row ID when no alias matches.
	InternalIDResolvable bool
	// ResolutionTarget marks kinds that may resolve another entity reference
	// (the historical validateResolutionTargetKind list).
	ResolutionTarget bool
}

// entityRegistry is the closed, explicitly ordered entity-kind registry.
// Legacy kinds are retained through explicit registration, not a wildcard;
// iteration order matches the historical internal-ID resolution order with
// new kinds appended after it.
var entityRegistry = []entityDescriptor{
	{Kind: "spec", Table: "specs", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "task", Table: "tasks", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "idea", Table: "ideas", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "spark", Table: "sparks", InternalIDResolvable: true},
	{Kind: "brainstorm", Table: "brainstorms", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "shaping_draft", Table: "shaping_drafts", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "report", Table: "reports", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "finding", Table: "findings", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "verdict", Table: "verdicts", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "run", Table: "runs", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "plan", Table: "plans", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "handoff", Table: "handoffs", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "council", Table: "councils", InternalIDResolvable: true, ResolutionTarget: true},
	{Kind: "journal_entry", Table: "journal_entries", InternalIDResolvable: true},
	{Kind: "intent", Table: "intents", InternalIDResolvable: true},
	{Kind: "exploration", Table: "explorations", InternalIDResolvable: true},
	{Kind: "exploration_checkpoint", Table: "exploration_checkpoints", InternalIDResolvable: true},
	{Kind: "logical_conversation", Table: "logical_conversations", InternalIDResolvable: true},
}

func entityDescriptorForKind(kind string) (entityDescriptor, bool) {
	for _, descriptor := range entityRegistry {
		if descriptor.Kind == kind {
			return descriptor, true
		}
	}
	return entityDescriptor{}, false
}

// registeredEntityTable returns the backing table for a registered kind and ""
// for anything unregistered, preserving the historical traceTable contract.
func registeredEntityTable(kind string) string {
	descriptor, ok := entityDescriptorForKind(kind)
	if !ok {
		return ""
	}
	return descriptor.Table
}

func internalIDResolvableKinds() []string {
	kinds := make([]string, 0, len(entityRegistry))
	for _, descriptor := range entityRegistry {
		if descriptor.InternalIDResolvable {
			kinds = append(kinds, descriptor.Kind)
		}
	}
	return kinds
}

func registeredEntityKinds() []string {
	kinds := make([]string, 0, len(entityRegistry))
	for _, descriptor := range entityRegistry {
		kinds = append(kinds, descriptor.Kind)
	}
	return kinds
}

// relationshipPairing is one supported directed relationship in the closed
// matrix. Anything not registered here — or in the legacy pairing set — is an
// explicit future extension, never an accidental write.
type relationshipPairing struct {
	FromKind string
	Type     string
	ToKind   string
}

// intentExplorationRelationshipMatrix is the bounded initial matrix accepted
// by the intent-exploration-foundation Change. Git Change references and
// materialized-as edges deliberately begin in change-native-execution-migration.
var intentExplorationRelationshipMatrix = []relationshipPairing{
	{FromKind: "spark", Type: "source-of", ToKind: "intent"},
	{FromKind: "idea", Type: "source-of", ToKind: "intent"},
	{FromKind: "brainstorm", Type: "source-of", ToKind: "intent"},
	{FromKind: "journal_entry", Type: "source-of", ToKind: "intent"},
	{FromKind: "intent", Type: "derived-from", ToKind: "intent"},
	{FromKind: "intent", Type: "split-from", ToKind: "intent"},
	{FromKind: "intent", Type: "supersedes", ToKind: "intent"},
	{FromKind: "intent", Type: "duplicates", ToKind: "intent"},
	{FromKind: "exploration", Type: "explores", ToKind: "intent"},
	{FromKind: "exploration", Type: "informs", ToKind: "intent"},
	{FromKind: "exploration", Type: "has-conversation", ToKind: "logical_conversation"},
	{FromKind: "exploration", Type: "uses-source", ToKind: "journal_entry"},
	{FromKind: "exploration", Type: "uses-source", ToKind: "handoff"},
	{FromKind: "exploration", Type: "uses-source", ToKind: "report"},
	{FromKind: "exploration", Type: "uses-source", ToKind: "finding"},
	{FromKind: "report", Type: "evidence-for", ToKind: "exploration"},
	{FromKind: "report", Type: "evidence-for", ToKind: "intent"},
	{FromKind: "finding", Type: "evidence-for", ToKind: "exploration"},
	{FromKind: "finding", Type: "evidence-for", ToKind: "intent"},
	{FromKind: "journal_entry", Type: "evidence-for", ToKind: "exploration"},
	{FromKind: "journal_entry", Type: "evidence-for", ToKind: "intent"},
}

// newEntityKinds are the kinds introduced by the Intent/Exploration foundation.
// A relationship write that touches any of them must match the closed matrix;
// writes between purely legacy kinds keep their historical behavior.
var newEntityKinds = map[string]bool{
	"intent":                 true,
	"exploration":            true,
	"exploration_checkpoint": true,
	"logical_conversation":   true,
}

func relationshipPairingSupported(fromKind, relationshipType, toKind string) bool {
	for _, pairing := range intentExplorationRelationshipMatrix {
		if pairing.FromKind == fromKind && pairing.Type == relationshipType && pairing.ToKind == toKind {
			return true
		}
	}
	return false
}

// validateRelationshipAgainstRegistry rejects relationship writes that touch a
// new entity kind unless the exact (from, type, to) pairing is registered.
// Legacy-to-legacy writes are not constrained here; their validation remains
// with the historical link surface.
func validateRelationshipAgainstRegistry(fromKind, relationshipType, toKind string) error {
	if !newEntityKinds[fromKind] && !newEntityKinds[toKind] {
		return nil
	}
	if _, ok := entityDescriptorForKind(fromKind); !ok {
		return fmt.Errorf("relationship source kind %q is not registered", fromKind)
	}
	if _, ok := entityDescriptorForKind(toKind); !ok {
		return fmt.Errorf("relationship target kind %q is not registered", toKind)
	}
	if !relationshipPairingSupported(fromKind, relationshipType, toKind) {
		return fmt.Errorf("relationship %s -[%s]-> %s is not in the supported matrix; supported pairings are explicit registrations, not a cartesian product", fromKind, relationshipType, toKind)
	}
	return nil
}

// supportedRelationshipTypesForNewKinds lists the distinct relationship types
// in the closed matrix, for diagnostics and fixtures.
func supportedRelationshipTypesForNewKinds() []string {
	seen := map[string]bool{}
	for _, pairing := range intentExplorationRelationshipMatrix {
		seen[pairing.Type] = true
	}
	types := make([]string, 0, len(seen))
	for relationshipType := range seen {
		types = append(types, relationshipType)
	}
	sort.Strings(types)
	return types
}

// checkpointItemTypes is the closed initial vocabulary for typed exploration
// checkpoint items.
var checkpointItemTypes = map[string]bool{
	"candidate": true,
	"evidence":  true,
}

func validateCheckpointItemType(itemType string) error {
	if !checkpointItemTypes[itemType] {
		return fmt.Errorf("checkpoint item type %q is not registered; supported types are candidate and evidence", itemType)
	}
	return nil
}

// nextAggregateSeq transactionally allocates the next immutable per-aggregate
// sequence. Callers must run inside a serializable transaction; the UNIQUE
// (aggregate, seq) constraint turns a lost race into a visible conflict
// instead of two rows claiming the same position. Table and column names are
// compile-time constants at every call site, never caller input.
func nextAggregateSeq(ctx context.Context, tx *sql.Tx, table, aggregateColumn, aggregateID string) (int, error) {
	var next int
	query := fmt.Sprintf(`SELECT COALESCE(MAX(seq), 0) + 1 FROM %s WHERE %s = ?`, table, aggregateColumn)
	if err := tx.QueryRowContext(ctx, query, aggregateID).Scan(&next); err != nil {
		return 0, fmt.Errorf("allocate %s sequence for %s: %w", table, aggregateID, err)
	}
	return next, nil
}
