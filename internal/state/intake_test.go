package state

import (
	"context"
	"fmt"
	"testing"
)

func TestCreateLinkEnforcesClosedMatrixForNewKinds(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	spark, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "seed"})
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	intent, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Linked", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	task, err := store.CreateTask(ctx, root, TaskCreateOptions{Title: "legacy task"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	// Supported matrix pairing round-trips through link and trace.
	link, err := store.CreateLink(ctx, root, LinkMutationOptions{From: spark.Spark.Alias, To: intent.Intent.Alias, Type: "source-of"})
	if err != nil {
		t.Fatalf("CreateLink(spark source-of intent) error = %v", err)
	}
	if link.From.Kind != "spark" || link.To.Kind != "intent" {
		t.Fatalf("link = %#v, want spark -> intent", link)
	}
	trace, err := store.Trace(ctx, root, intent.Intent.Alias)
	if err != nil {
		t.Fatalf("Trace(intent) error = %v", err)
	}
	if trace.Entity.Kind != "intent" || len(trace.Relationships) != 1 || trace.Relationships[0].Entity.Kind != "spark" {
		t.Fatalf("trace = %#v, want intent with one inbound spark edge", trace)
	}

	// Unsupported pairings touching a new kind fail visibly.
	if _, err := store.CreateLink(ctx, root, LinkMutationOptions{From: task.Task.Alias, To: intent.Intent.Alias, Type: "source-of"}); err == nil {
		t.Fatal("task source-of intent link accepted, want closed-matrix rejection")
	}
	if _, err := store.CreateLink(ctx, root, LinkMutationOptions{From: intent.Intent.Alias, To: spark.Spark.Alias, Type: "evidence-for"}); err == nil {
		t.Fatal("intent evidence-for spark link accepted, want closed-matrix rejection")
	}
	// Legacy-to-legacy links keep their historical open behavior.
	if _, err := store.CreateLink(ctx, root, LinkMutationOptions{From: spark.Spark.Alias, To: task.Task.Alias, Type: "relates_to"}); err != nil {
		t.Fatalf("legacy link error = %v, want preserved open behavior", err)
	}
}

func TestIntakeListShowsEachLogicalItemOnceAndIsDeterministic(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()

	if _, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "plain spark"}); err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	if _, err := store.CaptureIdea(ctx, root, IdeaCaptureOptions{Title: "an idea"}); err != nil {
		t.Fatalf("CaptureIdea() error = %v", err)
	}
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Tracked direction", Body: "b"}); err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	resolved, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Resolved direction", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent(resolved) error = %v", err)
	}
	if _, err := ResolveIntent(ctx, root, resolver, IntentDispositionOptions{IntentRef: resolved.Intent.Alias, Reason: "done"}); err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	// An adapter-era deferral carries its canonical intent; it must show once
	// as an intent, never additionally as a spark or legacy deferral.
	if _, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "adapter deferral", Why: "w", Boundary: "b", Trigger: "t", OperationID: "intake-op-1",
	}); err != nil {
		t.Fatalf("DeferJournal() error = %v", err)
	}
	// A pre-conversion legacy deferral (no canonical mapping) surfaces as one
	// legacy item.
	projectID := projectIDForTest(t, store, root)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES ('journal:legacy-intake', ?, 'decision', 'defer/x', 'Intent: legacy body', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')
`, projectID)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_search (rowid, journal_entry_id, project_id, session_id, entry_type, scope, message)
SELECT rowid, id, project_id, '', 'decision', 'defer/x', 'Intent: legacy body' FROM journal_entries WHERE id = 'journal:legacy-intake'
`)
	mustExecSchemaSQL(t, store, `
INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at)
VALUES ('spark:legacy-intake', ?, 'defer/x', 'open', 'Intent: legacy body' || char(10) || 'Decision: journal:legacy-intake', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')
`, projectID)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, 'legacy-intake-op', 'journal:legacy-intake', 'spark:legacy-intake', ?, '2026-07-01T00:00:00Z')
`, projectID, testDigest)

	first, err := ListIntake(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ListIntake() error = %v", err)
	}
	counts := map[string]int{}
	for _, item := range first.Items {
		counts[item.Kind]++
		if item.ReadCommand == "" {
			t.Fatalf("intake item %#v missing read command", item)
		}
	}
	want := map[string]int{"spark": 1, "idea": 1, "intent": 2, "legacy_deferral": 1}
	for kind, expected := range want {
		if counts[kind] != expected {
			t.Fatalf("intake counts = %v, want %v", counts, want)
		}
	}
	for _, item := range first.Items {
		if item.Kind == "intent" && item.Title == "Resolved direction" {
			t.Fatal("resolved intent appears in intake")
		}
		if item.Kind == "legacy_deferral" && item.Title != "legacy body" {
			t.Fatalf("legacy deferral title = %q, want packet Intent line", item.Title)
		}
	}

	second, err := ListIntake(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ListIntake() again error = %v", err)
	}
	if fmt.Sprintf("%#v", first.Items) != fmt.Sprintf("%#v", second.Items) {
		t.Fatal("intake projection is not deterministic for equal state")
	}
}

func TestExportCoversIntentAndExplorationTables(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Exported", Body: "b", Disposition: "deferred",
		Why: "w", Boundary: "b", Trigger: "t", OperationID: "export-op",
	}); err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	exploration, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Exported exploration"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	if _, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: exploration.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
	}); err != nil {
		t.Fatalf("AppendExplorationCheckpoint() error = %v", err)
	}

	snapshot, err := ExportAllJSON(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}
	for _, table := range []string{"intents", "intent_snapshots", "intent_deferrals", "intent_dispositions", "intent_operations", "explorations", "exploration_checkpoints"} {
		rows, ok := snapshot.Tables[table]
		if !ok {
			t.Fatalf("export snapshot missing table %s", table)
		}
		if len(rows) == 0 {
			t.Fatalf("export snapshot table %s is empty, want seeded rows", table)
		}
	}
}
