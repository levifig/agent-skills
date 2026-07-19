package state

import (
	"context"
	"strings"
	"testing"
)

// intentSchemaFixture initializes an isolated store and returns it with the
// registered project ID. Raw SQL is used deliberately: these tests falsify the
// migration 0012 constraints themselves, beneath any Go write path.
func intentSchemaFixture(t *testing.T) (*Store, string) {
	t.Helper()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, projectIDForTest(t, store, root)
}

func execSchemaSQL(t *testing.T, store *Store, query string, args ...any) error {
	t.Helper()
	_, err := store.db.ExecContext(context.Background(), query, args...)
	return err
}

func mustExecSchemaSQL(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	if err := execSchemaSQL(t, store, query, args...); err != nil {
		compact := strings.Join(strings.Fields(query), " ")
		if len(compact) > 60 {
			compact = compact[:60]
		}
		t.Fatalf("exec %q: %v", compact, err)
	}
}

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func seedIntent(t *testing.T, store *Store, projectID, intentID string) {
	t.Helper()
	mustExecSchemaSQL(t, store, `INSERT INTO intents (id, project_id, created_at) VALUES (?, ?, '2026-07-19T00:00:00Z')`, intentID, projectID)
}

func seedDeferral(t *testing.T, store *Store, projectID, intentID, deferralID, operationKey string) {
	t.Helper()
	mustExecSchemaSQL(t, store, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES (?, ?, ?, ?, 'body', 'why', 'boundary', 'trigger', ?, '2026-07-19T00:00:00Z')
`, deferralID, projectID, intentID, operationKey, testDigest)
}

func TestIntentSequenceUniquenessRejectsConcurrentDuplicates(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	seedIntent(t, store, projectID, "intent:a")

	mustExecSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES ('disp:1', ?, 'intent:a', 1, 'tracked', '2026-07-19T00:00:00Z')
`, projectID)
	err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES ('disp:2', ?, 'intent:a', 1, 'resolved', '2026-07-19T00:00:00Z')
`, projectID)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate (intent_id, seq) error = %v, want UNIQUE violation", err)
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO intent_snapshots (id, project_id, intent_id, seq, title, body, content_digest, created_at)
VALUES ('snap:1', ?, 'intent:a', 1, 'title', 'body', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	err = execSchemaSQL(t, store, `
INSERT INTO intent_snapshots (id, project_id, intent_id, seq, title, body, content_digest, created_at)
VALUES ('snap:2', ?, 'intent:a', 1, 'other', 'body', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate snapshot (intent_id, seq) error = %v, want UNIQUE violation", err)
	}
}

func TestIntentDispositionVocabularyAndDeferralPairingAreEnforced(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	seedIntent(t, store, projectID, "intent:a")
	seedDeferral(t, store, projectID, "intent:a", "deferral:1", "op-1")

	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES ('disp:bad-vocab', ?, 'intent:a', 1, 'paused', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("unknown disposition error = %v, want CHECK violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES ('disp:bad-deferred', ?, 'intent:a', 1, 'deferred', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("deferred without deferral_id error = %v, want CHECK violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, deferral_id, created_at)
VALUES ('disp:bad-tracked', ?, 'intent:a', 1, 'tracked', 'deferral:1', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("tracked with deferral_id error = %v, want CHECK violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, supersedes_deferral_id, created_at)
VALUES ('disp:bad-supersede', ?, 'intent:a', 1, 'resolved', 'deferral:1', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("resolved with supersedes_deferral_id error = %v, want CHECK violation", err)
	}
	mustExecSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, deferral_id, created_at)
VALUES ('disp:good-deferred', ?, 'intent:a', 1, 'deferred', 'deferral:1', '2026-07-19T00:00:00Z')
`, projectID)
	mustExecSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, supersedes_deferral_id, created_at)
VALUES ('disp:good-resume', ?, 'intent:a', 2, 'tracked', 'resumed', 'deferral:1', '2026-07-19T00:00:00Z')
`, projectID)
}

func TestIntentOperationsEnforceKeyUniquenessAndProjectionPairing(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	seedIntent(t, store, projectID, "intent:a")

	mustExecSchemaSQL(t, store, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, projection_version, created_at, updated_at)
VALUES (?, 'op-1', 'intent:a', ?, 0, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, projection_version, created_at, updated_at)
VALUES (?, 'op-1', 'intent:a', ?, 0, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil {
		t.Fatal("duplicate (project_id, operation_key) insert succeeded, want primary key violation")
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, journal_entry_id, spark_id, projection_version, created_at, updated_at)
VALUES (?, 'op-bad-v0', 'intent:a', ?, 'journal:x', 'spark:x', 0, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("projection version 0 with legacy IDs error = %v, want CHECK violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, projection_version, created_at, updated_at)
VALUES (?, 'op-bad-v1', 'intent:a', ?, 1, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("projection version 1 without legacy IDs error = %v, want CHECK violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, projection_version, created_at, updated_at)
VALUES (?, 'op-dangling', 'intent:missing', ?, 0, '2026-07-19T00:00:00Z', '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("dangling intent reference error = %v, want FOREIGN KEY violation", err)
	}
}

func TestIntentDeferralOperationKeyIsProjectScoped(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	seedIntent(t, store, projectID, "intent:a")
	seedIntent(t, store, projectID, "intent:b")
	seedDeferral(t, store, projectID, "intent:a", "deferral:1", "op-1")
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES ('deferral:2', ?, 'intent:b', 'op-1', 'body', 'why', 'boundary', 'trigger', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate deferral operation key error = %v, want UNIQUE violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES ('deferral:blank', ?, 'intent:a', 'op-2', '  ', 'why', 'boundary', 'trigger', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("blank deferral body error = %v, want CHECK violation", err)
	}
}

func TestExplorationCheckpointConstraints(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	mustExecSchemaSQL(t, store, `INSERT INTO explorations (id, project_id, title, created_at) VALUES ('expl:a', ?, 'inquiry', '2026-07-19T00:00:00Z')`, projectID)

	mustExecSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, operation_key, content_digest, created_at)
VALUES ('cp:1', ?, 'expl:a', 1, 'p', 'c', 'u', 'n', 'op-cp-1', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	if err := execSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at)
VALUES ('cp:dup-seq', ?, 'expl:a', 1, 'p', 'c', 'u', 'n', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate checkpoint seq error = %v, want UNIQUE violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, operation_key, content_digest, created_at)
VALUES ('cp:dup-op', ?, 'expl:a', 2, 'p', 'c', 'u', 'n', 'op-cp-1', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate checkpoint operation key error = %v, want UNIQUE violation", err)
	}
	// NULL operation keys never collide with one another.
	mustExecSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at)
VALUES ('cp:2', ?, 'expl:a', 2, 'p', 'c', 'u', 'n', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	mustExecSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at)
VALUES ('cp:3', ?, 'expl:a', 3, 'p', 'c', 'u', 'n', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest)
	if err := execSchemaSQL(t, store, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at)
VALUES ('cp:blank', ?, 'expl:a', 4, 'p', 'c', 'u', '   ', ?, '2026-07-19T00:00:00Z')
`, projectID, testDigest); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("blank next_action error = %v, want CHECK violation", err)
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO exploration_checkpoint_items (id, project_id, checkpoint_id, item_type, position, content, created_at)
VALUES ('item:1', ?, 'cp:1', 'candidate', 1, 'first', '2026-07-19T00:00:00Z')
`, projectID)
	if err := execSchemaSQL(t, store, `
INSERT INTO exploration_checkpoint_items (id, project_id, checkpoint_id, item_type, position, content, created_at)
VALUES ('item:dup', ?, 'cp:1', 'evidence', 1, 'second', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate item position error = %v, want UNIQUE violation", err)
	}
}

func TestConversationIdentityConstraints(t *testing.T) {
	store, projectID := intentSchemaFixture(t)
	mustExecSchemaSQL(t, store, `INSERT INTO logical_conversations (id, project_id, title, operation_key, created_at) VALUES ('conv:a', ?, 'thread', 'op-conv-1', '2026-07-19T00:00:00Z')`, projectID)
	if err := execSchemaSQL(t, store, `INSERT INTO logical_conversations (id, project_id, title, operation_key, created_at) VALUES ('conv:dup', ?, 'other', 'op-conv-1', '2026-07-19T00:00:00Z')`, projectID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate conversation operation key error = %v, want UNIQUE violation", err)
	}

	mustExecSchemaSQL(t, store, `
INSERT INTO conversation_handles (id, project_id, conversation_id, harness, handle, created_at)
VALUES ('handle:1', ?, 'conv:a', 'codex', 'opaque-1', '2026-07-19T00:00:00Z')
`, projectID)
	// NULL locality must still deduplicate through the COALESCE identity index.
	if err := execSchemaSQL(t, store, `
INSERT INTO conversation_handles (id, project_id, conversation_id, harness, handle, created_at)
VALUES ('handle:dup', ?, 'conv:a', 'codex', 'opaque-1', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate handle with NULL locality error = %v, want UNIQUE violation", err)
	}
	mustExecSchemaSQL(t, store, `
INSERT INTO conversation_handles (id, project_id, conversation_id, harness, handle, locality, created_at)
VALUES ('handle:2', ?, 'conv:a', 'codex', 'opaque-1', 'machine-b', '2026-07-19T00:00:00Z')
`, projectID)

	mustExecSchemaSQL(t, store, `
INSERT INTO conversation_log_refs (id, project_id, handle_id, locator, created_at)
VALUES ('log:1', ?, 'handle:1', '/logs/session.jsonl', '2026-07-19T00:00:00Z')
`, projectID)
	if err := execSchemaSQL(t, store, `
INSERT INTO conversation_log_refs (id, project_id, handle_id, locator, created_at)
VALUES ('log:dup', ?, 'handle:1', '/logs/session.jsonl', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate log ref error = %v, want UNIQUE violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO conversation_log_refs (id, project_id, handle_id, locator, content_hash, created_at)
VALUES ('log:badhash', ?, 'handle:1', '/logs/other.jsonl', 'nothex', '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("invalid content hash error = %v, want CHECK violation", err)
	}

	if err := execSchemaSQL(t, store, `
INSERT INTO source_availability_observations (id, project_id, subject_kind, subject_id, observed_at, available, created_at)
VALUES ('obs:bad', ?, 'session', 'handle:1', '2026-07-19T00:00:00Z', 1, '2026-07-19T00:00:00Z')
`, projectID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("unknown observation subject kind error = %v, want CHECK violation", err)
	}
	mustExecSchemaSQL(t, store, `
INSERT INTO source_availability_observations (id, project_id, subject_kind, subject_id, observed_at, observer, locality, available, created_at)
VALUES ('obs:1', ?, 'conversation_handle', 'handle:1', '2026-07-19T00:00:00Z', 'probe', 'machine-a', 0, '2026-07-19T00:00:00Z')
`, projectID)
}

// TestCrossProjectReferencesFailBeforeCommit proves the same-project pairing
// acceptance at the schema level: composite (project_id, ref) foreign keys
// reject a row in one project that references an aggregate owned by another.
func TestCrossProjectReferencesFailBeforeCommit(t *testing.T) {
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(context.Background(), rootA, resolver)
	if err != nil {
		t.Fatalf("Initialize(rootA) error = %v", err)
	}
	if _, err := Initialize(context.Background(), rootB, resolver); err != nil {
		t.Fatalf("Initialize(rootB) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	projectA := projectIDForTest(t, store, rootA)
	projectB := projectIDForTest(t, store, rootB)
	if projectA == projectB {
		t.Fatal("fixture projects must be distinct")
	}

	seedIntent(t, store, projectA, "intent:a")
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES ('deferral:cross', ?, 'intent:a', 'op-cross', 'body', 'why', 'boundary', 'trigger', ?, '2026-07-19T00:00:00Z')
`, projectB, testDigest); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-project deferral error = %v, want FOREIGN KEY violation", err)
	}
	if err := execSchemaSQL(t, store, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, created_at)
VALUES ('disp:cross', ?, 'intent:a', 1, 'tracked', '2026-07-19T00:00:00Z')
`, projectB); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-project disposition error = %v, want FOREIGN KEY violation", err)
	}

	mustExecSchemaSQL(t, store, `INSERT INTO explorations (id, project_id, title, created_at) VALUES ('expl:a', ?, 'inquiry', '2026-07-19T00:00:00Z')`, projectA)
	mustExecSchemaSQL(t, store, `INSERT INTO logical_conversations (id, project_id, title, created_at) VALUES ('conv:b', ?, 'thread', '2026-07-19T00:00:00Z')`, projectB)
	if err := execSchemaSQL(t, store, `
INSERT INTO exploration_conversations (id, project_id, exploration_id, conversation_id, created_at)
VALUES ('membership:cross', ?, 'expl:a', 'conv:b', '2026-07-19T00:00:00Z')
`, projectA); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("cross-project membership error = %v, want FOREIGN KEY violation", err)
	}
}

// TestIntentExplorationSchemaHasNoLifecycleState is the schema-inspection gate
// from the Change Verification Contract: the new tables must never grow a
// mutable status column or a current-session/current-exploration pointer.
func TestIntentExplorationSchemaHasNoLifecycleState(t *testing.T) {
	migration := SchemaMigrations()[len(SchemaMigrations())-1]
	if migration.Name != "intents_and_explorations" {
		t.Fatalf("last migration = %q, want intents_and_explorations", migration.Name)
	}
	for _, table := range []string{
		"intents", "intent_snapshots", "intent_deferrals", "intent_dispositions",
		"intent_operations", "explorations", "exploration_checkpoints",
		"exploration_checkpoint_items", "logical_conversations",
		"conversation_handles", "conversation_log_refs",
		"exploration_conversations", "journal_conversation_handles",
		"source_availability_observations",
	} {
		body := tableBody(t, migration.SQL, table)
		for _, line := range strings.Split(body, "\n") {
			trimmed := strings.TrimSpace(line)
			fields := strings.Fields(trimmed)
			if len(fields) == 0 {
				continue
			}
			column := strings.ToLower(fields[0])
			switch {
			case column == "status" || strings.HasSuffix(column, "_status"):
				t.Fatalf("%s declares lifecycle column %q", table, column)
			case strings.HasPrefix(column, "current_") || column == "active" || strings.HasSuffix(column, "_active"):
				t.Fatalf("%s declares current-pointer column %q", table, column)
			case column == "updated_at" && table != "intent_operations":
				t.Fatalf("%s declares updated_at on an append-only fact table", table)
			}
		}
	}
}
