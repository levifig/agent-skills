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

func intentTestFixture(t *testing.T) (project.Root, PathResolver, *Store) {
	t.Helper()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return root, resolver, store
}

func intentTableCounts(t *testing.T, store *Store) string {
	t.Helper()
	counts := []string{}
	for _, table := range []string{"intents", "intent_snapshots", "intent_deferrals", "intent_dispositions", "intent_operations", "aliases", "relationships"} {
		var count int
		if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts = append(counts, fmt.Sprintf("%s=%d", table, count))
	}
	return strings.Join(counts, " ")
}

func TestCreateIntentTrackedWritesSnapshotDispositionAndAlias(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()

	result, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Track journal-source navigation",
		Body:  "Origin pointers exist; decide whether navigation is worth building.",
	})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if !result.Created || result.Intent.Disposition != "tracked" || result.Intent.SnapshotSeq != 1 || result.Intent.DispositionSeq != 1 {
		t.Fatalf("result = %#v, want created tracked seq 1/1", result)
	}
	if !strings.HasPrefix(result.Intent.Alias, "INTENT-") {
		t.Fatalf("alias = %q, want INTENT- prefix", result.Intent.Alias)
	}
	if result.Intent.Deferral != nil {
		t.Fatalf("tracked create carries deferral %#v", result.Intent.Deferral)
	}

	show, err := store2Show(t, root, store, result.Intent.Alias)
	if err != nil {
		t.Fatalf("ShowIntent() error = %v", err)
	}
	if show.Intent.ID != result.Intent.ID || show.Intent.Disposition != "tracked" || show.Intent.Body != result.Intent.Body {
		t.Fatalf("show = %#v, want same intent tracked", show.Intent)
	}
}

// store2Show resolves ShowIntent against the same fixture database.
func store2Show(t *testing.T, root project.Root, store *Store, ref string) (IntentShowResult, error) {
	t.Helper()
	ctx := context.Background()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IntentShowResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IntentShowResult{}, err
	}
	defer tx.Rollback()
	intentID, _, err := resolveIntentRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return IntentShowResult{}, err
	}
	detail, err := loadIntentDetailTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentShowResult{}, err
	}
	return IntentShowResult{Query: ref, Intent: detail}, nil
}

func TestCreateIntentDeferredWritesImmutablePayloadAndMapping(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()

	result, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "Defer FTS artifact redaction",
		Body:        "Ingest-time secret exclusion for artifact_search remains unimplemented.",
		Disposition: "deferred",
		Why:         "Not required for the current foundation",
		Boundary:    "Excluded by the intent-exploration-foundation scope",
		Trigger:     "Revisit when search work resumes",
		OperationID: "defer-fts-redaction",
	})
	if err != nil {
		t.Fatalf("CreateIntent(deferred) error = %v", err)
	}
	if !result.Created || result.Intent.Disposition != "deferred" || result.Intent.Deferral == nil {
		t.Fatalf("result = %#v, want created deferred with payload", result)
	}
	if result.Intent.Deferral.Body != result.Intent.Body {
		t.Fatalf("deferral body %q != intent body %q", result.Intent.Deferral.Body, result.Intent.Body)
	}

	var version int
	var journalID, sparkID sql.NullString
	if err := store.db.QueryRowContext(ctx, `
SELECT projection_version, journal_entry_id, spark_id FROM intent_operations
WHERE operation_key = 'defer-fts-redaction'
`).Scan(&version, &journalID, &sparkID); err != nil {
		t.Fatalf("read operation mapping: %v", err)
	}
	if version != 0 || journalID.Valid || sparkID.Valid {
		t.Fatalf("mapping = v%d journal=%v spark=%v, want canonical-first v0 with null legacy IDs", version, journalID, sparkID)
	}
}

func TestIntentOperationRetryConvergesAcrossEntryPoints(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()

	first, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "Converge deferral entry points",
		Body:        "One canonical operation mapping.",
		Disposition: "deferred",
		Why:         "why", Boundary: "boundary", Trigger: "trigger",
		OperationID: "op-converge",
	})
	if err != nil {
		t.Fatalf("first CreateIntent error = %v", err)
	}

	// Identical retry through the same entry point returns the first write.
	retry, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "Converge deferral entry points",
		Body:        "One canonical operation mapping.",
		Disposition: "deferred",
		Why:         "why", Boundary: "boundary", Trigger: "trigger",
		OperationID: "op-converge",
	})
	if err != nil {
		t.Fatalf("retry CreateIntent error = %v", err)
	}
	if retry.Created || retry.Intent.ID != first.Intent.ID || !retry.InputDigestMatches {
		t.Fatalf("retry = %#v, want reused first intent with matching digest", retry)
	}

	// A reworded retry through the OTHER entry point converges on the mapped
	// intent and reports the digest mismatch instead of duplicating.
	reworded, err := store.DeferIntent(ctx, root, IntentDeferOptions{
		IntentRef: first.Intent.Alias,
		Why:       "different why", Boundary: "boundary", Trigger: "trigger",
		OperationID: "op-converge",
	})
	if err != nil {
		t.Fatalf("reworded DeferIntent error = %v", err)
	}
	if reworded.Created || reworded.Intent.ID != first.Intent.ID {
		t.Fatalf("reworded = %#v, want reused first intent", reworded)
	}
	if reworded.InputDigestMatches {
		t.Fatal("reworded retry reports digest match, want mismatch")
	}

	var intents int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intents`).Scan(&intents); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	if intents != 1 {
		t.Fatalf("intents = %d, want exactly one canonical intent", intents)
	}
}

func TestConcurrentIntentDeferralsWithOneKeyConvergeOnFirstWrite(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()

	const writers = 8
	results := make([]IntentMutationResult, writers)
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = store.CreateIntent(ctx, root, IntentCreateOptions{
				Title:       "Concurrent deferral",
				Body:        fmt.Sprintf("Body wording variant %d.", i),
				Disposition: "deferred",
				Why:         "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-concurrent",
			})
		}(i)
	}
	wg.Wait()

	created := 0
	var canonicalID string
	for i := 0; i < writers; i++ {
		if errs[i] != nil {
			t.Fatalf("writer %d error = %v", i, errs[i])
		}
		if results[i].Created {
			created++
			canonicalID = results[i].Intent.ID
		}
	}
	if created != 1 {
		t.Fatalf("created count = %d, want exactly 1 first write", created)
	}
	for i := 0; i < writers; i++ {
		if results[i].Intent.ID != canonicalID {
			t.Fatalf("writer %d intent %s, want canonical %s", i, results[i].Intent.ID, canonicalID)
		}
	}
	var intents, deferrals int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*), (SELECT COUNT(*) FROM intent_deferrals) FROM intents`).Scan(&intents, &deferrals); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if intents != 1 || deferrals != 1 {
		t.Fatalf("intents=%d deferrals=%d, want 1/1", intents, deferrals)
	}
}

func TestIntentCreateFailureInjectionLeavesNoPartialState(t *testing.T) {
	stages := []struct {
		name string
		hook func(*intentWriteHooks, error)
	}{
		{"after intent", func(h *intentWriteHooks, err error) { h.afterIntent = func(*sql.Tx) error { return err } }},
		{"after snapshot", func(h *intentWriteHooks, err error) { h.afterSnapshot = func(*sql.Tx) error { return err } }},
		{"after deferral", func(h *intentWriteHooks, err error) { h.afterDeferral = func(*sql.Tx) error { return err } }},
		{"after disposition", func(h *intentWriteHooks, err error) { h.afterDisposition = func(*sql.Tx) error { return err } }},
		{"after relationships", func(h *intentWriteHooks, err error) { h.afterRelationship = func(*sql.Tx) error { return err } }},
		{"after operation", func(h *intentWriteHooks, err error) { h.afterOperation = func(*sql.Tx) error { return err } }},
		{"before commit", func(h *intentWriteHooks, err error) { h.beforeCommit = func(*sql.Tx) error { return err } }},
	}
	for _, stage := range stages {
		t.Run(strings.ReplaceAll(stage.name, " ", "-"), func(t *testing.T) {
			root, _, store := intentTestFixture(t)
			ctx := context.Background()
			spark, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "seed spark"})
			if err != nil {
				t.Fatalf("CaptureSpark() error = %v", err)
			}
			before := intentTableCounts(t, store)

			hooks := &intentWriteHooks{}
			stage.hook(hooks, errors.New("injected failure"))
			_, err = store.createIntentWithHooks(ctx, root, IntentCreateOptions{
				Title:       "Failure injection",
				Body:        "Body.",
				Disposition: "deferred",
				Why:         "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-failure-" + stage.name,
				Sources:     []string{spark.Spark.Alias},
			}, hooks)
			if err == nil {
				t.Fatalf("stage %s: error = nil, want injected failure", stage.name)
			}
			var stageErr *IntentTransactionError
			if !errors.As(err, &stageErr) || stageErr.Stage != stage.name {
				t.Fatalf("stage %s: error = %v, want IntentTransactionError at that stage", stage.name, err)
			}
			after := intentTableCounts(t, store)
			if before != after {
				t.Fatalf("stage %s left partial state:\nbefore: %s\nafter:  %s", stage.name, before, after)
			}
		})
	}
}

func TestIntentDeferFailureInjectionLeavesNoPartialState(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	created, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Defer target", Body: "Body."})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	for _, stage := range []string{"after deferral", "after disposition", "after operation", "before commit"} {
		t.Run(strings.ReplaceAll(stage, " ", "-"), func(t *testing.T) {
			before := intentTableCounts(t, store)
			hooks := &intentWriteHooks{}
			injected := errors.New("injected failure")
			switch stage {
			case "after deferral":
				hooks.afterDeferral = func(*sql.Tx) error { return injected }
			case "after disposition":
				hooks.afterDisposition = func(*sql.Tx) error { return injected }
			case "after operation":
				hooks.afterOperation = func(*sql.Tx) error { return injected }
			case "before commit":
				hooks.beforeCommit = func(*sql.Tx) error { return injected }
			}
			_, err := store.deferIntentWithHooks(ctx, root, IntentDeferOptions{
				IntentRef: created.Intent.Alias,
				Why:       "why", Boundary: "boundary", Trigger: "trigger",
				OperationID: "op-defer-fail-" + stage,
			}, hooks)
			if err == nil {
				t.Fatalf("stage %s: error = nil, want injected failure", stage)
			}
			if after := intentTableCounts(t, store); after != before {
				t.Fatalf("stage %s left partial state:\nbefore: %s\nafter:  %s", stage, before, after)
			}
		})
	}
}

func TestIntentResumePreservesDeferralAndDerivesTracked(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()
	created, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:       "Resumable direction",
		Body:        "Body.",
		Disposition: "deferred",
		Why:         "why", Boundary: "boundary", Trigger: "trigger",
		OperationID: "op-resume",
	})
	if err != nil {
		t.Fatalf("CreateIntent(deferred) error = %v", err)
	}

	resumed, err := ResumeIntent(ctx, root, resolver, IntentDispositionOptions{
		IntentRef: created.Intent.Alias,
		Reason:    "capacity is available now",
	})
	if err != nil {
		t.Fatalf("ResumeIntent() error = %v", err)
	}
	if resumed.Intent.Disposition != "tracked" || resumed.Intent.DispositionSeq != 2 {
		t.Fatalf("resumed = %#v, want tracked seq 2", resumed.Intent)
	}

	// The immutable deferral payload and the deferred disposition history
	// survive; the resume links the deferral it supersedes.
	var payloadCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intent_deferrals WHERE intent_id = ?`, created.Intent.ID).Scan(&payloadCount); err != nil {
		t.Fatalf("count deferrals: %v", err)
	}
	if payloadCount != 1 {
		t.Fatalf("deferral payloads = %d, want preserved 1", payloadCount)
	}
	var supersedes string
	if err := store.db.QueryRowContext(ctx, `
SELECT supersedes_deferral_id FROM intent_dispositions WHERE intent_id = ? AND seq = 2
`, created.Intent.ID).Scan(&supersedes); err != nil {
		t.Fatalf("read resume disposition: %v", err)
	}
	if supersedes != created.Intent.Deferral.ID {
		t.Fatalf("supersedes = %q, want %q", supersedes, created.Intent.Deferral.ID)
	}

	// Resuming a non-deferred intent is rejected.
	if _, err := ResumeIntent(ctx, root, resolver, IntentDispositionOptions{IntentRef: created.Intent.Alias, Reason: "again"}); err == nil {
		t.Fatal("second resume succeeded, want rejection of non-deferred intent")
	}

	resolved, err := ResolveIntent(ctx, root, resolver, IntentDispositionOptions{
		IntentRef: created.Intent.Alias,
		Reason:    "shipped in the foundation change",
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	if resolved.Intent.Disposition != "resolved" || resolved.Intent.DispositionSeq != 3 {
		t.Fatalf("resolved = %#v, want resolved seq 3", resolved.Intent)
	}
	var history int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intent_dispositions WHERE intent_id = ?`, created.Intent.ID).Scan(&history); err != nil {
		t.Fatalf("count dispositions: %v", err)
	}
	if history != 3 {
		t.Fatalf("disposition history = %d rows, want all 3 preserved", history)
	}
}

func TestIntentCreateValidatesSourcesAndInput(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()

	// A task is not a supported source-of pairing for intent.
	taskResult, err := store.CreateTask(ctx, root, TaskCreateOptions{Title: "unsupported source"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:   "Invalid source",
		Body:    "Body.",
		Sources: []string{taskResult.Task.Alias},
	}); err == nil || !strings.Contains(err.Error(), "not in the supported matrix") {
		t.Fatalf("task source error = %v, want unsupported matrix rejection", err)
	}

	// Unknown sources fail visibly.
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:   "Dangling source",
		Body:    "Body.",
		Sources: []string{"SPARK-does-not-exist"},
	}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("dangling source error = %v, want not-found rejection", err)
	}

	// Control characters and oversize fields are rejected without writes.
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "bad\x00title", Body: "Body."}); err == nil {
		t.Fatal("control-character title accepted")
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "ok", Body: strings.Repeat("x", intentFieldMaxBytes+1)}); err == nil {
		t.Fatal("oversize body accepted")
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "ok", Body: "Body.", Disposition: "paused"}); err == nil {
		t.Fatal("unknown disposition accepted")
	}

	var intents int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intents`).Scan(&intents); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	if intents != 0 {
		t.Fatalf("intents = %d after rejected writes, want 0", intents)
	}
}

func TestIntentSourcesRecordSupportedPairings(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	spark, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "spark source"})
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}

	result, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title:   "Sourced intent",
		Body:    "Body.",
		Sources: []string{spark.Spark.Alias},
	})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if len(result.Intent.Sources) != 1 || result.Intent.Sources[0].Entity.Kind != "spark" || result.Intent.Sources[0].Type != "source-of" {
		t.Fatalf("sources = %#v, want one spark source-of edge", result.Intent.Sources)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE from_entity_kind = 'spark' AND to_entity_kind = 'intent' AND relationship_type = 'source-of'
`).Scan(&count); err != nil {
		t.Fatalf("count relationships: %v", err)
	}
	if count != 1 {
		t.Fatalf("relationship rows = %d, want 1", count)
	}
}

func TestListIntentsIsDeterministicAndFilters(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Alpha", Body: "Body."}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Beta", Body: "Body.", Disposition: "deferred",
		Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "op-beta",
	}); err != nil {
		t.Fatalf("create beta: %v", err)
	}

	all, err := ListIntents(ctx, root, resolver, "")
	if err != nil {
		t.Fatalf("ListIntents() error = %v", err)
	}
	if len(all.Intents) != 2 {
		t.Fatalf("intents = %d, want 2", len(all.Intents))
	}
	deferred, err := ListIntents(ctx, root, resolver, "deferred")
	if err != nil {
		t.Fatalf("ListIntents(deferred) error = %v", err)
	}
	if len(deferred.Intents) != 1 || deferred.Intents[0].Title != "Beta" {
		t.Fatalf("deferred list = %#v, want only Beta", deferred.Intents)
	}
	if _, err := ListIntents(ctx, root, resolver, "paused"); err == nil {
		t.Fatal("unknown disposition filter accepted")
	}

	again, err := ListIntents(ctx, root, resolver, "")
	if err != nil {
		t.Fatalf("ListIntents() again error = %v", err)
	}
	if fmt.Sprintf("%#v", again.Intents) != fmt.Sprintf("%#v", all.Intents) {
		t.Fatal("list projection is not deterministic for equal state")
	}
}
