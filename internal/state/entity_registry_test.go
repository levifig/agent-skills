package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestEntityRegistryCoversEveryKindWithBackingTable(t *testing.T) {
	schemaSQL := currentSchemaSQL()
	seen := map[string]bool{}
	for _, descriptor := range entityRegistry {
		if descriptor.Kind == "" || descriptor.Table == "" {
			t.Fatalf("registry descriptor %#v has empty kind or table", descriptor)
		}
		if seen[descriptor.Kind] {
			t.Fatalf("registry kind %q is registered twice", descriptor.Kind)
		}
		seen[descriptor.Kind] = true
		if got := traceTable(descriptor.Kind); got != descriptor.Table {
			t.Fatalf("traceTable(%q) = %q, want %q", descriptor.Kind, got, descriptor.Table)
		}
		if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS "+descriptor.Table+" (") {
			t.Fatalf("registry table %q for kind %q is not created by any auto-applied migration", descriptor.Table, descriptor.Kind)
		}
	}
	if traceTable("unregistered") != "" {
		t.Fatal("traceTable must return empty for unregistered kinds")
	}
	if err := validateResolutionTargetKind("intent", "INTENT-x"); err == nil {
		t.Fatal("intent must not be a resolution target kind")
	}
	if err := validateResolutionTargetKind("spec", "SPEC-001"); err != nil {
		t.Fatalf("spec resolution target error = %v, want nil", err)
	}
}

func TestRelationshipRegistryRejectsUnsupportedPairings(t *testing.T) {
	for _, pairing := range intentExplorationRelationshipMatrix {
		if err := validateRelationshipAgainstRegistry(pairing.FromKind, pairing.Type, pairing.ToKind); err != nil {
			t.Fatalf("supported pairing %v rejected: %v", pairing, err)
		}
	}
	for _, rejected := range []relationshipPairing{
		{FromKind: "task", Type: "source-of", ToKind: "intent"},
		{FromKind: "intent", Type: "evidence-for", ToKind: "spark"},
		{FromKind: "spark", Type: "explores", ToKind: "intent"},
		{FromKind: "intent", Type: "materialized-as", ToKind: "spec"},
		{FromKind: "exploration", Type: "uses-source", ToKind: "task"},
		{FromKind: "logical_conversation", Type: "has-conversation", ToKind: "exploration"},
		{FromKind: "unregistered", Type: "source-of", ToKind: "intent"},
	} {
		if err := validateRelationshipAgainstRegistry(rejected.FromKind, rejected.Type, rejected.ToKind); err == nil {
			t.Fatalf("unsupported pairing %v was accepted", rejected)
		}
	}
	// Legacy-to-legacy pairs remain outside the closed matrix.
	if err := validateRelationshipAgainstRegistry("spark", "resolved_by", "idea"); err != nil {
		t.Fatalf("legacy pairing rejected: %v", err)
	}
}

func TestValidateCheckpointItemType(t *testing.T) {
	for _, valid := range []string{"candidate", "evidence"} {
		if err := validateCheckpointItemType(valid); err != nil {
			t.Fatalf("validateCheckpointItemType(%q) = %v, want nil", valid, err)
		}
	}
	for _, invalid := range []string{"note", "transcript", "", "Candidate"} {
		if err := validateCheckpointItemType(invalid); err == nil {
			t.Fatalf("validateCheckpointItemType(%q) = nil, want error", invalid)
		}
	}
}

// TestNextAggregateSeqAllocatesDistinctSequencesUnderConcurrency proves the
// ordering acceptance: concurrent conflicting appends receive distinct
// committed per-aggregate sequences and the derived latest follows the
// greatest sequence, even when every row shares one timestamp.
func TestNextAggregateSeqAllocatesDistinctSequencesUnderConcurrency(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	seedIntent(t, store, projectID, "intent:seq")

	const writers = 10
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			ctx := context.Background()
			for attempt := 0; attempt < 200; attempt++ {
				tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
				if err != nil {
					continue
				}
				seq, err := nextAggregateSeq(ctx, tx, "intent_dispositions", "intent_id", "intent:seq")
				if err != nil {
					_ = tx.Rollback()
					continue
				}
				disposition := "tracked"
				if seq%2 == 0 {
					disposition = "resolved"
				}
				_, err = tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES (?, ?, 'intent:seq', ?, ?, '2026-07-19T00:00:00Z')
`, fmt.Sprintf("disp:writer-%d", writer), projectID, seq, disposition)
				if err != nil {
					_ = tx.Rollback()
					continue
				}
				if err := tx.Commit(); err != nil {
					continue
				}
				errs <- nil
				return
			}
			errs <- fmt.Errorf("writer %d exhausted retries", writer)
		}(writer)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	rows, err := store.db.QueryContext(context.Background(), `SELECT seq FROM intent_dispositions WHERE intent_id = 'intent:seq' ORDER BY seq`)
	if err != nil {
		t.Fatalf("read sequences: %v", err)
	}
	defer rows.Close()
	var sequences []int
	for rows.Next() {
		var seq int
		if err := rows.Scan(&seq); err != nil {
			t.Fatalf("scan sequence: %v", err)
		}
		sequences = append(sequences, seq)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sequences: %v", err)
	}
	if len(sequences) != writers {
		t.Fatalf("committed rows = %d, want %d", len(sequences), writers)
	}
	for i, seq := range sequences {
		if seq != i+1 {
			t.Fatalf("sequences = %v, want dense 1..%d", sequences, writers)
		}
	}

	// Derived latest must follow the greatest committed sequence even though
	// every row carries the identical created_at timestamp.
	var latest string
	if err := store.db.QueryRowContext(context.Background(), `
SELECT disposition FROM intent_dispositions
WHERE intent_id = 'intent:seq'
ORDER BY seq DESC LIMIT 1
`).Scan(&latest); err != nil {
		t.Fatalf("read derived latest: %v", err)
	}
	want := "tracked"
	if writers%2 == 0 {
		want = "resolved"
	}
	if latest != want {
		t.Fatalf("derived latest disposition = %q, want %q (greatest sequence %d)", latest, want, writers)
	}
}
