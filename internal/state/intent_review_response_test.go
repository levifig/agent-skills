package state

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

// The tests in this file falsify the fixes applied in response to the
// independent units 1-4 review and close its named coverage gaps.

func TestDeferOperationKeyBoundToOtherIntentIsRejected(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	first, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "First", Body: "b", Disposition: "deferred",
		Why: "w", Boundary: "b", Trigger: "t", OperationID: "shared-key",
	})
	if err != nil {
		t.Fatalf("CreateIntent(first) error = %v", err)
	}
	other, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Other", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent(other) error = %v", err)
	}
	_, err = store.DeferIntent(ctx, root, IntentDeferOptions{
		IntentRef: other.Intent.Alias,
		Why:       "w", Boundary: "b", Trigger: "t",
		OperationID: "shared-key",
	})
	if err == nil || !strings.Contains(err.Error(), "already bound to intent "+first.Intent.ID) {
		t.Fatalf("cross-intent key reuse error = %v, want explicit binding rejection", err)
	}
	var dispositions int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intent_dispositions WHERE intent_id = ?`, other.Intent.ID).Scan(&dispositions); err != nil {
		t.Fatalf("count dispositions: %v", err)
	}
	if dispositions != 1 {
		t.Fatalf("other intent dispositions = %d, want unchanged 1", dispositions)
	}
}

func TestCheckpointOperationKeyBoundToOtherExplorationIsRejected(t *testing.T) {
	root, _, store := explorationFixture(t)
	ctx := context.Background()
	first, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "First"})
	if err != nil {
		t.Fatalf("CreateExploration(first) error = %v", err)
	}
	second, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Second"})
	if err != nil {
		t.Fatalf("CreateExploration(second) error = %v", err)
	}
	if _, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: first.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		OperationID: "shared-cp-key",
	}); err != nil {
		t.Fatalf("first checkpoint error = %v", err)
	}
	_, err = store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: second.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		OperationID: "shared-cp-key",
	})
	if err == nil || !strings.Contains(err.Error(), "already bound to a checkpoint on exploration "+first.Exploration.ID) {
		t.Fatalf("cross-exploration key reuse error = %v, want explicit binding rejection", err)
	}
	var checkpoints int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM exploration_checkpoints WHERE exploration_id = ?`, second.Exploration.ID).Scan(&checkpoints); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if checkpoints != 0 {
		t.Fatalf("second exploration checkpoints = %d, want 0", checkpoints)
	}
}

func TestJournalDeferOnTrackedIntentKeyReportsClearError(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	tracked, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Tracked with key", Body: "b", OperationID: "tracked-key",
	})
	if err != nil {
		t.Fatalf("CreateIntent(tracked) error = %v", err)
	}
	_, err = store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "b", Why: "w", Boundary: "b", Trigger: "t", OperationID: "tracked-key",
	})
	if err == nil || !strings.Contains(err.Error(), "already bound to intent "+tracked.Intent.ID) || !strings.Contains(err.Error(), "not deferred") {
		t.Fatalf("tracked-key adapter error = %v, want clear non-deferred binding explanation", err)
	}
	var journalEntries int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&journalEntries); err != nil {
		t.Fatalf("count journal entries: %v", err)
	}
	if journalEntries != 0 {
		t.Fatalf("journal entries after rejected adapter call = %d, want 0", journalEntries)
	}
}

// matrixFixture creates one entity of every kind the initial relationship
// matrix names and returns refs by kind.
func matrixFixture(t *testing.T, root project.Root, store *Store) map[string]string {
	t.Helper()
	ctx := context.Background()
	refs := map[string]string{}

	spark, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "matrix spark"})
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	refs["spark"] = spark.Spark.ID
	idea, err := store.CaptureIdea(ctx, root, IdeaCaptureOptions{Title: "matrix idea"})
	if err != nil {
		t.Fatalf("CaptureIdea() error = %v", err)
	}
	refs["idea"] = idea.Idea.ID
	brainstorm, err := store.CaptureBrainstorm(ctx, root, BrainstormCaptureOptions{Title: "matrix brainstorm", Body: "body"})
	if err != nil {
		t.Fatalf("CaptureBrainstorm() error = %v", err)
	}
	refs["brainstorm"] = brainstorm.Brainstorm.ID
	entry, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "discover(matrix): journal source"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	refs["journal_entry"] = entry.ID
	intent, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "matrix intent", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	refs["intent"] = intent.Intent.ID
	secondIntent, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "matrix intent two", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent(two) error = %v", err)
	}
	refs["intent2"] = secondIntent.Intent.ID
	exploration, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "matrix exploration"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	refs["exploration"] = exploration.Exploration.ID
	conversation, err := store.CreateConversation(ctx, root, ConversationCreateOptions{Title: "matrix conversation", OperationID: "matrix-conv"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	refs["logical_conversation"] = conversation.Conversation.ID
	handoff, err := store.CreateArtifactEntity(ctx, root, ArtifactEntityCreateOptions{Kind: "handoff", Title: "matrix handoff", Body: "body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	refs["handoff"] = handoff.Entity.ID
	report, err := store.CreateReport(ctx, root, ReportCreateOptions{Slug: "matrix-report", Kind: "review", Body: "body", SetBody: true})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}
	refs["report"] = report.Report.ID
	finding, err := store.CreateFinding(ctx, root, FindingCreateOptions{
		Report: report.Report.ID, Title: "matrix finding", Severity: "low", Confidence: "medium", Dimension: "correctness",
	})
	if err != nil {
		t.Fatalf("CreateFinding() error = %v", err)
	}
	refs["finding"] = finding.Finding.ID
	return refs
}

func TestFullMatrixLinkAndTraceRoundTrip(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	refs := matrixFixture(t, root, store)

	for _, pairing := range intentExplorationRelationshipMatrix {
		fromRef := refs[pairing.FromKind]
		toRef := refs[pairing.ToKind]
		if pairing.FromKind == "intent" && pairing.ToKind == "intent" {
			toRef = refs["intent2"]
		}
		if fromRef == "" || toRef == "" {
			t.Fatalf("fixture missing refs for pairing %v", pairing)
		}
		link, err := store.CreateLink(ctx, root, LinkMutationOptions{From: fromRef, To: toRef, Type: pairing.Type})
		if err != nil {
			t.Fatalf("CreateLink(%v) error = %v", pairing, err)
		}
		if link.From.Kind != pairing.FromKind || link.To.Kind != pairing.ToKind {
			t.Fatalf("link kinds = %s->%s, want %s->%s", link.From.Kind, link.To.Kind, pairing.FromKind, pairing.ToKind)
		}
		trace, err := store.Trace(ctx, root, toRef)
		if err != nil {
			t.Fatalf("Trace(%s %s) error = %v", pairing.ToKind, toRef, err)
		}
		found := false
		for _, relationship := range trace.Relationships {
			if relationship.Direction == "inbound" && relationship.Type == pairing.Type && relationship.Entity.ID == fromRef {
				found = true
			}
		}
		if !found {
			t.Fatalf("trace of %s %s missing inbound %s from %s", pairing.ToKind, toRef, pairing.Type, pairing.FromKind)
		}
	}
}

func seedFullIntentExplorationState(t *testing.T, root project.Root, store *Store) {
	t.Helper()
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)
	if _, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "seed intent", Body: "b", Disposition: "deferred",
		Why: "w", Boundary: "b", Trigger: "t", OperationID: "seed-op-" + projectID[:8],
	}); err != nil {
		t.Fatalf("seed intent: %v", err)
	}
	exploration, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "seed exploration"})
	if err != nil {
		t.Fatalf("seed exploration: %v", err)
	}
	if _, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: exploration.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		Items: []CheckpointItemInput{{Type: "candidate", Content: "x"}},
	}); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	conversation, err := store.CreateConversation(ctx, root, ConversationCreateOptions{Title: "seed conversation"})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	handle, err := store.AddConversationHandle(ctx, root, ConversationHandleAddOptions{
		ConversationRef: conversation.Conversation.ID,
		Harness:         "codex", Handle: "seed-handle", LogRef: "/tmp/seed.jsonl",
	})
	if err != nil {
		t.Fatalf("seed handle: %v", err)
	}
	if _, err := store.AddExplorationConversation(ctx, root, exploration.Exploration.Alias, conversation.Conversation.ID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	if _, err := store.ObserveConversationSource(ctx, root, ConversationObserveOptions{
		SubjectKind: "conversation_handle", SubjectID: handle.HandleID, Available: true,
	}); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	entry, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "discover(seed): handle association"})
	if err != nil {
		t.Fatalf("seed journal entry: %v", err)
	}
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_conversation_handles (id, project_id, journal_entry_id, handle_id, created_at)
VALUES (?, ?, ?, ?, '2026-07-19T00:00:00Z')
`, stableMigrationID("journal-conversation-handle", projectID, entry.ID, handle.HandleID), projectID, entry.ID, handle.HandleID)
}

var intentExplorationTables = []string{
	"intents", "intent_snapshots", "intent_deferrals", "intent_dispositions",
	"intent_operations", "explorations", "exploration_checkpoints",
	"exploration_checkpoint_items", "logical_conversations",
	"conversation_handles", "conversation_log_refs", "exploration_conversations",
	"journal_conversation_handles", "source_availability_observations",
}

func TestProjectDeletionRemovesSeededIntentExplorationRows(t *testing.T) {
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	statusA, err := Initialize(context.Background(), rootA, resolver)
	if err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	if _, err := Initialize(context.Background(), rootB, resolver); err != nil {
		t.Fatalf("Initialize(B) error = %v", err)
	}
	store, err := OpenStore(statusA.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	projectA := projectIDForTest(t, store, rootA)
	projectB := projectIDForTest(t, store, rootB)
	seedFullIntentExplorationState(t, rootA, store)
	seedFullIntentExplorationState(t, rootB, store)

	for _, table := range intentExplorationTables {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectA).Scan(&count); err != nil {
			t.Fatalf("precount %s: %v", table, err)
		}
		if count == 0 {
			t.Fatalf("fixture seeded no rows in %s; deletion coverage would be vacuous", table)
		}
	}

	if _, err := DeleteProject(ctx, rootB, resolver, projectA); err != nil {
		t.Fatalf("DeleteProject(A) error = %v", err)
	}
	for _, table := range intentExplorationTables {
		var removed, preserved int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectA).Scan(&removed); err != nil {
			t.Fatalf("count %s after delete: %v", table, err)
		}
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectB).Scan(&preserved); err != nil {
			t.Fatalf("count %s preserved: %v", table, err)
		}
		if removed != 0 {
			t.Fatalf("%s retains %d rows for the deleted project", table, removed)
		}
		if preserved == 0 {
			t.Fatalf("%s lost the other project's rows", table)
		}
	}
}

func TestBackupPreservesIntentExplorationRows(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()
	seedFullIntentExplorationState(t, root, store)

	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	restored, err := OpenStoreReadOnly(backup.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(backup) error = %v", err)
	}
	defer restored.Close()
	for _, table := range intentExplorationTables {
		var live, inBackup int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&live); err != nil {
			t.Fatalf("count live %s: %v", table, err)
		}
		if err := restored.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&inBackup); err != nil {
			t.Fatalf("count backup %s: %v", table, err)
		}
		if live == 0 {
			t.Fatalf("fixture seeded no rows in %s", table)
		}
		if live != inBackup {
			t.Fatalf("%s backup rows = %d, want %d", table, inBackup, live)
		}
	}
}

func TestExplorationContextIntentsLayerPaginates(t *testing.T) {
	root, resolver, store := explorationFixture(t)
	ctx := context.Background()
	exploration, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "many intents"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	for i := 0; i < 7; i++ {
		intent, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: fmt.Sprintf("linked %d", i), Body: "b"})
		if err != nil {
			t.Fatalf("CreateIntent(%d) error = %v", i, err)
		}
		if _, err := store.CreateLink(ctx, root, LinkMutationOptions{From: exploration.Exploration.ID, To: intent.Intent.ID, Type: "explores"}); err != nil {
			t.Fatalf("CreateLink(%d) error = %v", i, err)
		}
	}

	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		result, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{
			ExplorationRef: exploration.Exploration.Alias,
			Layer:          "intents",
			Cursor:         cursor,
			Limit:          3,
		})
		if err != nil {
			t.Fatalf("ExplorationContext(page %d) error = %v", pages, err)
		}
		layer := result.Layers["intents"]
		var page []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(layer.Items, &page); err != nil {
			t.Fatalf("decode intents page: %v", err)
		}
		for _, item := range page {
			if seen[item.ID] {
				t.Fatalf("intents pagination duplicated %s", item.ID)
			}
			seen[item.ID] = true
		}
		pages++
		if !layer.Truncated {
			break
		}
		cursor = layer.Cursor
	}
	if len(seen) != 7 || pages != 3 {
		t.Fatalf("intents pagination covered %d items in %d pages, want 7 in 3", len(seen), pages)
	}
}

// The three tests below falsify the Codex adversarial state-review findings.

func TestCanonicalCreateRejectsLegacyBoundOperationKey(t *testing.T) {
	root, resolver, store := intentTestFixture(t)
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)
	seedLegacyDeferral(t, store, projectID, "legacy-captured", "Intent: legacy body\nWhy: w\nBoundary: b\nTrigger: t")

	// A tracked canonical create must not capture the legacy key.
	_, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Unrelated tracked direction", Body: "b", OperationID: "legacy-captured",
	})
	if err == nil || !strings.Contains(err.Error(), "pre-conversion legacy deferral") {
		t.Fatalf("tracked create with legacy key error = %v, want legacy-binding rejection", err)
	}
	// A canonical defer of another intent must not capture it either.
	other, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: "Other", Body: "b"})
	if err != nil {
		t.Fatalf("CreateIntent(other) error = %v", err)
	}
	if _, err := store.DeferIntent(ctx, root, IntentDeferOptions{
		IntentRef: other.Intent.Alias, Why: "w", Boundary: "b", Trigger: "t", OperationID: "legacy-captured",
	}); err == nil || !strings.Contains(err.Error(), "pre-conversion legacy deferral") {
		t.Fatalf("defer with legacy key error = %v, want legacy-binding rejection", err)
	}

	// The legacy deferral stays visible in intake and convertible.
	intake, err := ListIntake(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ListIntake() error = %v", err)
	}
	legacyVisible := false
	for _, item := range intake.Items {
		if item.Kind == "legacy_deferral" && item.OperationKey == "legacy-captured" {
			legacyVisible = true
		}
	}
	if !legacyVisible {
		t.Fatal("legacy deferral disappeared from intake")
	}
	conversion, err := store.convertLegacyDeferrals(ctx, root, true, "test-backup")
	if err != nil {
		t.Fatalf("convertLegacyDeferrals() error = %v", err)
	}
	if conversion.Convertible != 1 {
		t.Fatalf("conversion convertible = %d, want 1", conversion.Convertible)
	}
}

func TestCheckpointRetryDigestCoversItems(t *testing.T) {
	root, _, store := intentTestFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Digest items"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	first, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		Items:       []CheckpointItemInput{{Type: "candidate", Content: "A"}},
		OperationID: "cp-items-1",
	})
	if err != nil {
		t.Fatalf("first checkpoint error = %v", err)
	}
	retry, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		Items:       []CheckpointItemInput{{Type: "candidate", Content: "B"}},
		OperationID: "cp-items-1",
	})
	if err != nil {
		t.Fatalf("retry checkpoint error = %v", err)
	}
	if retry.Created || retry.Checkpoint.ID != first.Checkpoint.ID {
		t.Fatalf("retry = %#v, want stored first write", retry)
	}
	if retry.InputDigestMatches {
		t.Fatal("retry with different items reported digest match, want mismatch")
	}
	identical, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		Items:       []CheckpointItemInput{{Type: "candidate", Content: "A"}},
		OperationID: "cp-items-1",
	})
	if err != nil {
		t.Fatalf("identical retry error = %v", err)
	}
	if !identical.InputDigestMatches {
		t.Fatal("identical retry reported digest mismatch, want match")
	}
}

func TestDispositionCannotReferenceAnotherIntentsDeferral(t *testing.T) {
	root, _, store := intentTestFixture(t)
	projectID := projectIDForTest(t, store, root)
	seedIntent(t, store, projectID, "intent:a")
	seedIntent(t, store, projectID, "intent:b")
	seedDeferral(t, store, projectID, "intent:b", "deferral:b", "op-b")

	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, deferral_id, created_at)
VALUES ('disp:cross-intent', ?, 'intent:a', 1, 'deferred', 'deferral:b', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-intent deferral reference error = %v, want FOREIGN KEY violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, supersedes_deferral_id, created_at)
VALUES ('disp:cross-supersede', ?, 'intent:a', 1, 'tracked', 'deferral:b', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-intent supersedes reference error = %v, want FOREIGN KEY violation", err)
	}
}
