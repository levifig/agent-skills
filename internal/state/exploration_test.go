package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func explorationFixture(t *testing.T) (project.Root, PathResolver, *Store) {
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
	return root, resolver, store
}

func seedPortableExploration(t *testing.T, root project.Root, store *Store) ExplorationMutationResult {
	t.Helper()
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Workflow and artifact model"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	checkpointed, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "Understand the portable context contract",
		Conclusions:    "Append-only facts beat lifecycle flags",
		Unresolved:     "How should availability observations shape triage?",
		NextAction:     "Implement the context projection and hand it to a fresh agent",
		Items: []CheckpointItemInput{
			{Type: "candidate", Content: "bounded layers with cursors"},
			{Type: "evidence", Content: "journal defer convergence tests"},
		},
	})
	if err != nil {
		t.Fatalf("AppendExplorationCheckpoint() error = %v", err)
	}
	return checkpointed
}

func TestExplorationCheckpointPortableCoreValidation(t *testing.T) {
	root, _, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Validation target"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}

	base := ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
	}
	for field, mutate := range map[string]func(*ExplorationCheckpointOptions){
		"purpose":     func(o *ExplorationCheckpointOptions) { o.Purpose = "  " },
		"conclusions": func(o *ExplorationCheckpointOptions) { o.Conclusions = "" },
		"unresolved":  func(o *ExplorationCheckpointOptions) { o.Unresolved = "\n\t" },
		"next_action": func(o *ExplorationCheckpointOptions) { o.NextAction = " " },
	} {
		options := base
		mutate(&options)
		_, err := store.AppendExplorationCheckpoint(ctx, root, options)
		var validation *ExplorationValidationError
		if err == nil || !errors.As(err, &validation) || validation.Field != field {
			t.Fatalf("missing %s error = %v, want field-specific validation error", field, err)
		}
	}

	// Oversize core fields fail with the stable field error and leave no rows.
	oversize := base
	oversize.NextAction = strings.Repeat("x", checkpointFieldMaxBytes+1)
	if _, err := store.AppendExplorationCheckpoint(ctx, root, oversize); err == nil || !strings.Contains(err.Error(), "next_action") || !strings.Contains(err.Error(), "4096") {
		t.Fatalf("oversize next_action error = %v, want stable byte-cap error", err)
	}
	// Multibyte input just over the cap is rejected on bytes, not runes.
	multibyte := base
	multibyte.Purpose = strings.Repeat("é", checkpointFieldMaxBytes/2+1)
	if _, err := store.AppendExplorationCheckpoint(ctx, root, multibyte); err == nil || !strings.Contains(err.Error(), "purpose") {
		t.Fatalf("multibyte oversize error = %v, want purpose byte-cap error", err)
	}
	var checkpoints, items int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*), (SELECT COUNT(*) FROM exploration_checkpoint_items) FROM exploration_checkpoints`).Scan(&checkpoints, &items); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if checkpoints != 0 || items != 0 {
		t.Fatalf("checkpoints=%d items=%d after rejected writes, want 0/0", checkpoints, items)
	}

	// A checkpoint exactly at the cap is accepted whole, never truncated.
	exact := base
	exact.NextAction = strings.Repeat("y", checkpointFieldMaxBytes)
	result, err := store.AppendExplorationCheckpoint(ctx, root, exact)
	if err != nil {
		t.Fatalf("exact-cap checkpoint error = %v", err)
	}
	if len(result.Checkpoint.NextAction) != checkpointFieldMaxBytes {
		t.Fatalf("stored next_action bytes = %d, want %d untruncated", len(result.Checkpoint.NextAction), checkpointFieldMaxBytes)
	}
}

func TestHandleOnlyExplorationReportsNoPortableContext(t *testing.T) {
	root, resolver, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Handle-only exploration"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	conversation, err := store.CreateConversation(ctx, root, ConversationCreateOptions{Title: "codex thread", OperationID: "conv-1"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := store.AddConversationHandle(ctx, root, ConversationHandleAddOptions{
		ConversationRef: conversation.Conversation.ID,
		Harness:         "codex",
		Handle:          "019f62e6-88d2-7630-96ce-527652fd9a0b",
		Locality:        "machine-a",
	}); err != nil {
		t.Fatalf("AddConversationHandle() error = %v", err)
	}
	if _, err := store.AddExplorationConversation(ctx, root, created.Exploration.Alias, conversation.Conversation.ID); err != nil {
		t.Fatalf("AddExplorationConversation() error = %v", err)
	}

	result, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{ExplorationRef: created.Exploration.Alias})
	if err != nil {
		t.Fatalf("ExplorationContext() error = %v", err)
	}
	if result.PortableContextPresent {
		t.Fatal("handle-only exploration claims portable context")
	}
	if result.Checkpoint != nil {
		t.Fatalf("handle-only exploration returned checkpoint %#v", result.Checkpoint)
	}
	conversations := result.Layers["conversations"]
	if conversations.Available != 1 || conversations.Shown != 1 {
		t.Fatalf("conversations layer = %#v, want the one source handle visible", conversations)
	}
}

func TestExplorationContextSurvivesUnavailableSources(t *testing.T) {
	root, resolver, store := explorationFixture(t)
	ctx := context.Background()
	checkpointed := seedPortableExploration(t, root, store)
	explorationRef := checkpointed.Exploration.Alias
	wantNext := checkpointed.Checkpoint.NextAction

	conversation, err := store.CreateConversation(ctx, root, ConversationCreateOptions{Title: "primary thread", OperationID: "conv-primary"})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	handle, err := store.AddConversationHandle(ctx, root, ConversationHandleAddOptions{
		ConversationRef: conversation.Conversation.ID,
		Harness:         "codex",
		Handle:          "local-thread-1",
		Locality:        "machine-a",
		LogRef:          "/Users/nobody/.codex/sessions/thread.jsonl",
		Hash:            strings.Repeat("a", 64),
		Range:           "1-500",
	})
	if err != nil {
		t.Fatalf("AddConversationHandle() error = %v", err)
	}
	if _, err := store.AddExplorationConversation(ctx, root, explorationRef, conversation.Conversation.ID); err != nil {
		t.Fatalf("AddExplorationConversation() error = %v", err)
	}
	// Every local source is observed unavailable.
	if _, err := store.ObserveConversationSource(ctx, root, ConversationObserveOptions{
		SubjectKind: "conversation_handle", SubjectID: handle.HandleID, Available: false, Observer: "probe", Locality: "machine-b", Note: "log purged",
	}); err != nil {
		t.Fatalf("ObserveConversationSource(handle) error = %v", err)
	}
	if _, err := store.ObserveConversationSource(ctx, root, ConversationObserveOptions{
		SubjectKind: "conversation_log_ref", SubjectID: handle.LogRefID, Available: false, Observer: "probe", Locality: "machine-b",
	}); err != nil {
		t.Fatalf("ObserveConversationSource(log) error = %v", err)
	}

	result, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{ExplorationRef: explorationRef})
	if err != nil {
		t.Fatalf("ExplorationContext() error = %v", err)
	}
	if !result.PortableContextPresent || result.Checkpoint == nil {
		t.Fatalf("context = %#v, want portable checkpoint present", result)
	}
	if result.Checkpoint.NextAction != wantNext {
		t.Fatalf("next action = %q, want byte-identical %q with every source unavailable", result.Checkpoint.NextAction, wantNext)
	}
	var conversationsLayer []ConversationDetail
	if err := json.Unmarshal(result.Layers["conversations"].Items, &conversationsLayer); err != nil {
		t.Fatalf("decode conversations layer: %v", err)
	}
	if len(conversationsLayer) != 1 || len(conversationsLayer[0].Handles) != 1 {
		t.Fatalf("conversations layer = %#v, want one conversation with one handle", conversationsLayer)
	}
	observed := conversationsLayer[0].Handles[0]
	if observed.Latest == nil || observed.Latest.Available || observed.Latest.Locality != "machine-b" {
		t.Fatalf("handle observation = %#v, want latest unavailable at machine-b", observed.Latest)
	}
	if len(observed.LogRefs) != 1 || observed.LogRefs[0].Latest == nil || observed.LogRefs[0].Latest.Available {
		t.Fatalf("log ref observation = %#v, want latest unavailable", observed.LogRefs)
	}

	// The handle row itself never mutates when availability changes.
	var handleColumns int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('conversation_handles') WHERE name IN ('available', 'availability', 'status', 'updated_at')`).Scan(&handleColumns); err != nil {
		t.Fatalf("inspect handle columns: %v", err)
	}
	if handleColumns != 0 {
		t.Fatalf("conversation_handles carries %d mutable availability columns, want 0", handleColumns)
	}
}

func TestExplorationContextLayersPaginateDeterministically(t *testing.T) {
	root, resolver, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Paginated"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	items := make([]CheckpointItemInput, 25)
	for i := range items {
		items[i] = CheckpointItemInput{Type: "candidate", Content: fmt.Sprintf("candidate %02d", i+1)}
	}
	if _, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		Items: items,
	}); err != nil {
		t.Fatalf("AppendExplorationCheckpoint() error = %v", err)
	}

	collected := []CheckpointItemDetail{}
	cursor := ""
	pages := 0
	for {
		result, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{
			ExplorationRef: created.Exploration.Alias,
			Layer:          "items",
			Cursor:         cursor,
			Limit:          10,
		})
		if err != nil {
			t.Fatalf("ExplorationContext(page %d) error = %v", pages, err)
		}
		layer := result.Layers["items"]
		var page []CheckpointItemDetail
		if err := json.Unmarshal(layer.Items, &page); err != nil {
			t.Fatalf("decode page %d: %v", pages, err)
		}
		collected = append(collected, page...)
		pages++
		if layer.Available != 25 {
			t.Fatalf("page %d available = %d, want 25", pages, layer.Available)
		}
		if !layer.Truncated {
			break
		}
		if layer.Cursor == "" || !strings.Contains(layer.ExpandCommand, "--layer items --cursor ") {
			t.Fatalf("truncated layer without cursor/expand command: %#v", layer)
		}
		cursor = layer.Cursor
	}
	if pages != 3 || len(collected) != 25 {
		t.Fatalf("pages=%d collected=%d, want 3 pages covering all 25", pages, len(collected))
	}
	for i, item := range collected {
		if item.Position != i+1 {
			t.Fatalf("pagination omitted or duplicated: item %d has position %d", i, item.Position)
		}
	}

	// Cursors are layer- and exploration-bound.
	if _, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{
		ExplorationRef: created.Exploration.Alias,
		Layer:          "conversations",
		Cursor:         cursor,
		Limit:          10,
	}); err == nil {
		t.Fatal("cursor accepted for the wrong layer")
	}
	if _, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{
		ExplorationRef: created.Exploration.Alias,
		Layer:          "items",
		Cursor:         "not-a-cursor",
		Limit:          10,
	}); err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

func TestConcurrentCheckpointAppendsReceiveDistinctSequences(t *testing.T) {
	root, resolver, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Concurrent"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}

	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for attempt := 0; attempt < 50; attempt++ {
				_, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
					ExplorationRef: created.Exploration.Alias,
					Purpose:        fmt.Sprintf("writer %d", i),
					Conclusions:    "c", Unresolved: "u",
					NextAction: fmt.Sprintf("next from writer %d", i),
				})
				if err == nil {
					errs[i] = nil
					return
				}
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d error = %v", i, err)
		}
	}

	rows, err := store.db.QueryContext(ctx, `SELECT seq FROM exploration_checkpoints WHERE exploration_id = ? ORDER BY seq`, created.Exploration.ID)
	if err != nil {
		t.Fatalf("read sequences: %v", err)
	}
	defer rows.Close()
	var sequences []int
	for rows.Next() {
		var seq int
		if err := rows.Scan(&seq); err != nil {
			t.Fatalf("scan: %v", err)
		}
		sequences = append(sequences, seq)
	}
	if len(sequences) != writers {
		t.Fatalf("checkpoints = %d, want %d", len(sequences), writers)
	}
	for i, seq := range sequences {
		if seq != i+1 {
			t.Fatalf("sequences = %v, want dense 1..%d", sequences, writers)
		}
	}
	context, err := ExplorationContext(ctx, root, resolver, ExplorationContextOptions{ExplorationRef: created.Exploration.Alias})
	if err != nil {
		t.Fatalf("ExplorationContext() error = %v", err)
	}
	if context.Checkpoint == nil || context.Checkpoint.Seq != writers {
		t.Fatalf("context checkpoint = %#v, want greatest committed seq %d", context.Checkpoint, writers)
	}
}

func TestCheckpointOperationKeyRetryReturnsFirstWrite(t *testing.T) {
	root, _, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Retry"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	first, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
		OperationID: "cp-op-1",
	})
	if err != nil {
		t.Fatalf("first checkpoint error = %v", err)
	}
	retry, err := store.AppendExplorationCheckpoint(ctx, root, ExplorationCheckpointOptions{
		ExplorationRef: created.Exploration.Alias,
		Purpose:        "reworded", Conclusions: "c", Unresolved: "u", NextAction: "n",
		OperationID: "cp-op-1",
	})
	if err != nil {
		t.Fatalf("retry checkpoint error = %v", err)
	}
	if retry.Created || retry.Checkpoint.ID != first.Checkpoint.ID || retry.InputDigestMatches {
		t.Fatalf("retry = %#v, want first write with digest mismatch", retry)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM exploration_checkpoints`).Scan(&count); err != nil {
		t.Fatalf("count checkpoints: %v", err)
	}
	if count != 1 {
		t.Fatalf("checkpoints = %d, want 1", count)
	}
}

func TestExplorationSchemaStoresNoTranscriptBodies(t *testing.T) {
	// The provenance tables store locators, hashes, and bounded ranges — no
	// column can hold transcript/prompt/tool/provider payload bodies.
	migration := SchemaMigrations()[len(SchemaMigrations())-1]
	for _, table := range []string{"logical_conversations", "conversation_handles", "conversation_log_refs", "source_availability_observations"} {
		body := tableBody(t, migration.SQL, table)
		for _, forbidden := range []string{"transcript", "prompt", "payload", "content TEXT", "body TEXT", "message TEXT"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s declares %q; provenance tables must not store bodies", table, forbidden)
			}
		}
	}
}

func TestExplorationCheckpointFailureInjectionLeavesNoPartialState(t *testing.T) {
	root, _, store := explorationFixture(t)
	ctx := context.Background()
	created, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Failure injection"})
	if err != nil {
		t.Fatalf("CreateExploration() error = %v", err)
	}
	for _, stage := range []string{"after checkpoint", "after items", "before commit"} {
		t.Run(strings.ReplaceAll(stage, " ", "-"), func(t *testing.T) {
			hooks := &explorationWriteHooks{}
			injected := fmt.Errorf("injected failure")
			switch stage {
			case "after checkpoint":
				hooks.afterCheckpoint = func(*sql.Tx) error { return injected }
			case "after items":
				hooks.afterItems = func(*sql.Tx) error { return injected }
			case "before commit":
				hooks.beforeCommit = func(*sql.Tx) error { return injected }
			}
			_, err := store.appendExplorationCheckpointWithHooks(ctx, root, ExplorationCheckpointOptions{
				ExplorationRef: created.Exploration.Alias,
				Purpose:        "p", Conclusions: "c", Unresolved: "u", NextAction: "n",
				Items: []CheckpointItemInput{{Type: "candidate", Content: "x"}},
			}, hooks)
			if err == nil {
				t.Fatalf("stage %s: error = nil, want injected failure", stage)
			}
			var checkpoints, items int
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*), (SELECT COUNT(*) FROM exploration_checkpoint_items) FROM exploration_checkpoints`).Scan(&checkpoints, &items); err != nil {
				t.Fatalf("count rows: %v", err)
			}
			if checkpoints != 0 || items != 0 {
				t.Fatalf("stage %s left checkpoints=%d items=%d, want 0/0", stage, checkpoints, items)
			}
		})
	}
}
