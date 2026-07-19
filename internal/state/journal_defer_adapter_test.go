package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func adapterFixture(t *testing.T) (project.Root, *Store, string) {
	t.Helper()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return root, store, status.DatabasePath
}

func TestJournalDeferAfterCanonicalFirstBackfillsProjections(t *testing.T) {
	root, store, path := adapterFixture(t)
	ctx := context.Background()

	canonical, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "Canonical-first deferral",
		Body:        "canonical body",
		Disposition: "deferred",
		Why:         "canonical why", Boundary: "canonical boundary", Trigger: "canonical trigger",
		OperationID: "op-canonical-first",
	})
	if err != nil {
		t.Fatalf("CreateIntent(deferred) error = %v", err)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("journal entries before adapter = %d, want 0", got)
	}

	// The adapter retry uses DIFFERENT wording; projections must come from
	// the stored canonical packet, not this retry body.
	result, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "retry wording", Why: "retry why", Boundary: "retry boundary", Trigger: "retry trigger",
		OperationID: "op-canonical-first",
	})
	if err != nil {
		t.Fatalf("DeferJournal(backfill) error = %v", err)
	}
	if result.Created {
		t.Fatal("backfill reported Created = true, want reuse of the canonical first write")
	}
	if result.IntentID != canonical.Intent.ID || result.IntentAlias != canonical.Intent.Alias {
		t.Fatalf("backfill intent = %q/%q, want %q/%q", result.IntentID, result.IntentAlias, canonical.Intent.ID, canonical.Intent.Alias)
	}
	if result.Decision.ID == "" || result.Spark.ID == "" {
		t.Fatalf("backfill result = %#v, want established legacy IDs", result)
	}
	if result.InputDigestMatches {
		t.Fatal("backfill digest match = true, want mismatch for reworded retry")
	}
	wantPacket := "Intent: canonical body\nWhy: canonical why\nBoundary: canonical boundary\nTrigger: canonical trigger"
	if !strings.HasPrefix(result.Decision.Message, wantPacket) {
		t.Fatalf("decision message %q does not carry the stored canonical packet", result.Decision.Message)
	}
	if strings.Contains(result.Decision.Message, "retry why") {
		t.Fatal("decision message carries retry wording, want stored canonical content only")
	}

	var version int
	var journalID, sparkID string
	if err := store.db.QueryRowContext(ctx, `
SELECT projection_version, journal_entry_id, spark_id FROM intent_operations WHERE operation_key = 'op-canonical-first'
`).Scan(&version, &journalID, &sparkID); err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	if version != 1 || journalID != result.Decision.ID || sparkID != result.Spark.ID {
		t.Fatalf("mapping = v%d %q/%q, want v1 with established pair", version, journalID, sparkID)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM intents`); got != 1 {
		t.Fatalf("intents = %d, want the single canonical intent", got)
	}

	// A second adapter retry now takes the established-pair path.
	again, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "canonical body", Why: "canonical why", Boundary: "canonical boundary", Trigger: "canonical trigger",
		OperationID: "op-canonical-first",
	})
	if err != nil {
		t.Fatalf("DeferJournal(second retry) error = %v", err)
	}
	if again.Created || again.Decision.ID != result.Decision.ID || again.Spark.ID != result.Spark.ID || !again.InputDigestMatches {
		t.Fatalf("second retry = %#v, want established pair with digest match", again)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("journal entries after second retry = %d, want 1", got)
	}
}

func TestCanonicalAfterAdapterFirstReusesMappedIntent(t *testing.T) {
	root, store, path := adapterFixture(t)
	ctx := context.Background()

	adapter, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "adapter-first body", Why: "why", Boundary: "boundary", Trigger: "trigger",
		OperationID: "op-adapter-first",
	})
	if err != nil {
		t.Fatalf("DeferJournal(adapter-first) error = %v", err)
	}
	if !adapter.Created || adapter.IntentID == "" || adapter.IntentAlias == "" {
		t.Fatalf("adapter-first = %#v, want created with canonical intent identity", adapter)
	}

	// A later canonical call with the same key reuses the mapped intent and
	// leaves the existing projections untouched.
	canonical, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "different title",
		Body:        "different body",
		Disposition: "deferred",
		Why:         "different why", Boundary: "different boundary", Trigger: "different trigger",
		OperationID: "op-adapter-first",
	})
	if err != nil {
		t.Fatalf("CreateIntent(retry) error = %v", err)
	}
	if canonical.Created || canonical.Intent.ID != adapter.IntentID {
		t.Fatalf("canonical retry = %#v, want reuse of adapter-first intent %q", canonical, adapter.IntentID)
	}
	if canonical.InputDigestMatches {
		t.Fatal("canonical reworded retry digest match = true, want mismatch")
	}

	var version int
	var journalID, sparkID string
	if err := store.db.QueryRowContext(ctx, `
SELECT projection_version, journal_entry_id, spark_id FROM intent_operations WHERE operation_key = 'op-adapter-first'
`).Scan(&version, &journalID, &sparkID); err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	if version != 1 || journalID != adapter.Decision.ID || sparkID != adapter.Spark.ID {
		t.Fatalf("mapping = v%d %q/%q, want unchanged adapter projections", version, journalID, sparkID)
	}
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM intents`:          1,
		`SELECT COUNT(*) FROM journal_entries`:  1,
		`SELECT COUNT(*) FROM sparks`:           1,
		`SELECT COUNT(*) FROM intent_deferrals`: 1,
	} {
		if got := rawCount(t, path, query); got != want {
			t.Fatalf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestConcurrentMixedEntryPointsConvergeOnOneIntentAndOnePair(t *testing.T) {
	root, store, path := adapterFixture(t)
	ctx := context.Background()

	const writers = 8
	var wg sync.WaitGroup
	intentIDs := make([]string, writers)
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				result, err := store.CreateIntent(ctx, root, IntentCreateOptions{
					Title:       "Mixed entry",
					Body:        "mixed body",
					Disposition: "deferred",
					Why:         "why", Boundary: "boundary", Trigger: "trigger",
					OperationID: "op-mixed",
				})
				intentIDs[i], errs[i] = result.Intent.ID, err
				return
			}
			result, err := store.DeferJournal(ctx, root, JournalDeferOptions{
				Intent: "mixed body", Why: "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-mixed",
			})
			intentIDs[i], errs[i] = result.IntentID, err
		}(i)
	}
	wg.Wait()

	for i := 0; i < writers; i++ {
		if errs[i] != nil {
			t.Fatalf("writer %d error = %v", i, errs[i])
		}
	}
	canonical := intentIDs[0]
	for i := 1; i < writers; i++ {
		if intentIDs[i] != canonical {
			t.Fatalf("writer %d intent %q, want converged %q", i, intentIDs[i], canonical)
		}
	}
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM intents`:                                        1,
		`SELECT COUNT(*) FROM intent_deferrals`:                               1,
		`SELECT COUNT(*) FROM intent_operations`:                              1,
		`SELECT COUNT(*) FROM intent_operations WHERE projection_version = 1`: 1,
		`SELECT COUNT(*) FROM journal_entries`:                                1,
		`SELECT COUNT(*) FROM sparks`:                                         1,
		`SELECT COUNT(*) FROM journal_deferrals`:                              1,
	} {
		if got := rawCount(t, path, query); got != want {
			t.Fatalf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestBackfillProjectionAdvanceIsAllOrNothing(t *testing.T) {
	stages := []struct {
		name string
		set  func(*journalDeferHooks, error)
	}{
		{"after decision", func(h *journalDeferHooks, err error) { h.afterDecision = func(*sql.Tx) error { return err } }},
		{"after FTS", func(h *journalDeferHooks, err error) { h.afterFTS = func(*sql.Tx) error { return err } }},
		{"after spark", func(h *journalDeferHooks, err error) { h.afterSpark = func(*sql.Tx) error { return err } }},
		{"after alias/event", func(h *journalDeferHooks, err error) { h.afterAliasEvent = func(*sql.Tx) error { return err } }},
		{"after deferral", func(h *journalDeferHooks, err error) { h.afterDeferral = func(*sql.Tx) error { return err } }},
		{"after canonical intent", func(h *journalDeferHooks, err error) { h.afterCanonicalIntent = func(*sql.Tx) error { return err } }},
		{"before commit", func(h *journalDeferHooks, err error) { h.beforeCommit = func(*sql.Tx) error { return err } }},
	}
	for _, stage := range stages {
		t.Run(strings.ReplaceAll(stage.name, " ", "-"), func(t *testing.T) {
			root, store, path := adapterFixture(t)
			ctx := context.Background()
			if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
				Title:       "Backfill target",
				Body:        "body",
				Disposition: "deferred",
				Why:         "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-backfill",
			}); err != nil {
				t.Fatalf("CreateIntent(deferred) error = %v", err)
			}

			countState := func() string {
				parts := []string{}
				for _, query := range []string{
					`SELECT COUNT(*) FROM journal_entries`,
					`SELECT COUNT(*) FROM sparks`,
					`SELECT COUNT(*) FROM journal_deferrals`,
					`SELECT COUNT(*) FROM intent_operations WHERE projection_version = 1`,
					`SELECT COUNT(*) FROM intent_operations WHERE projection_version = 0`,
				} {
					parts = append(parts, fmt.Sprintf("%d", rawCount(t, path, query)))
				}
				return strings.Join(parts, "/")
			}
			before := countState()
			if before != "0/0/0/0/1" {
				t.Fatalf("precondition = %s, want 0/0/0/0/1", before)
			}

			hooks := &journalDeferHooks{}
			stage.set(hooks, errors.New("injected failure"))
			_, err := store.deferJournalWithHooks(ctx, root, JournalDeferOptions{
				Intent: "body", Why: "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-backfill",
			}, hooks)
			if err == nil {
				t.Fatalf("stage %s: error = nil, want injected failure", stage.name)
			}
			if after := countState(); after != before {
				t.Fatalf("stage %s left partial backfill: before=%s after=%s", stage.name, before, after)
			}

			// After the failed backfill, a clean retry completes 0→1 exactly once.
			result, err := store.DeferJournal(ctx, root, JournalDeferOptions{
				Intent: "body", Why: "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-backfill",
			})
			if err != nil {
				t.Fatalf("clean retry error = %v", err)
			}
			if result.Created || result.Decision.ID == "" || result.Spark.ID == "" || !result.InputDigestMatches {
				t.Fatalf("clean retry = %#v, want established projections with digest match", result)
			}
			if got := countState(); got != "1/1/1/1/0" {
				t.Fatalf("state after clean retry = %s, want 1/1/1/1/0", got)
			}
		})
	}
}

func TestLegacyOnlyDeferralRetryStaysLegacyUntilConversion(t *testing.T) {
	root, store, path := adapterFixture(t)
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)

	// Simulate a pre-migration legacy deferral: journal_deferrals pair with no
	// canonical mapping, as real databases hold before explicit conversion.
	packet := "Intent: legacy body\nWhy: legacy why\nBoundary: legacy boundary\nTrigger: legacy trigger"
	digest := intentDigest(packet)
	decisionID := stableMigrationID("journal-defer-decision", projectID, "op-legacy")
	sparkID := stableMigrationID("journal-defer-spark", projectID, "op-legacy")
	now := "2026-07-01T00:00:00Z"
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES (?, ?, 'decision', 'defer/legacy', ?, ?, ?)
`, decisionID, projectID, packet+"\nSpark: "+sparkID, now, now)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_search (rowid, journal_entry_id, project_id, session_id, entry_type, scope, message)
SELECT rowid, id, project_id, '', 'decision', 'defer/legacy', ? FROM journal_entries WHERE id = ?
`, packet+"\nSpark: "+sparkID, decisionID)
	mustExecSchemaSQL(t, store, `
INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at)
VALUES (?, ?, 'defer/legacy', 'open', ?, ?, ?)
`, sparkID, projectID, packet+"\nDecision: "+decisionID, now, now)
	mustExecSchemaSQL(t, store, `INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at) VALUES ('alias:legacy', ?, 'spark', ?, 'spark', 'SPARK-DEFER-legacy', ?, ?)`, projectID, sparkID, now, now)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, 'op-legacy', ?, ?, ?, ?)
`, projectID, decisionID, sparkID, digest, now)

	retry, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "legacy body", Why: "legacy why", Boundary: "legacy boundary", Trigger: "legacy trigger",
		OperationID: "op-legacy",
	})
	if err != nil {
		t.Fatalf("DeferJournal(legacy retry) error = %v", err)
	}
	if retry.Created || retry.Decision.ID != decisionID || retry.Spark.ID != sparkID || !retry.InputDigestMatches {
		t.Fatalf("legacy retry = %#v, want established legacy pair", retry)
	}
	if retry.IntentID != "" {
		t.Fatalf("legacy retry intent = %q, want empty until explicit conversion", retry.IntentID)
	}
	if got := rawCount(t, path, `SELECT COUNT(*) FROM intents`); got != 0 {
		t.Fatalf("intents after legacy retry = %d, want 0 (no implicit conversion)", got)
	}
}
