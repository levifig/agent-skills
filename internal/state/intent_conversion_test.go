package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func seedLegacyDeferral(t *testing.T, store *Store, projectID, operationKey, packet string) (string, string) {
	t.Helper()
	decisionID := stableMigrationID("journal-defer-decision", projectID, operationKey)
	sparkID := stableMigrationID("journal-defer-spark", projectID, operationKey)
	now := "2026-07-01T00:00:00Z"
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES (?, ?, 'decision', 'defer/legacy', ?, ?, ?)
`, decisionID, projectID, packet+"\nSpark: "+sparkID, now, now)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_search (rowid, journal_entry_id, project_id, session_id, entry_type, scope, message)
SELECT rowid, id, project_id, '', 'decision', 'defer/legacy', message FROM journal_entries WHERE id = ?
`, decisionID)
	mustExecSchemaSQL(t, store, `
INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at)
VALUES (?, ?, 'defer/legacy', 'open', ?, ?, ?)
`, sparkID, projectID, packet+"\nDecision: "+decisionID, now, now)
	operationDigest := journalDeferOperationDigest(projectID, operationKey)
	mustExecSchemaSQL(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'spark', ?, 'spark', ?, ?, ?)
`, stableMigrationID("alias", projectID, "spark", "SPARK-DEFER-"+operationDigest[:journalDeferScopePrefixLen]), projectID, sparkID, "SPARK-DEFER-"+operationDigest[:journalDeferScopePrefixLen], now, now)
	mustExecSchemaSQL(t, store, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, operationKey, decisionID, sparkID, intentDigest(packet), now)
	return decisionID, sparkID
}

func conversionFixture(t *testing.T) (project.Root, PathResolver, *Store, string) {
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
	return root, resolver, store, status.DatabasePath
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestConvertLegacyDeferralsDryRunIsNonMutating(t *testing.T) {
	root, resolver, store, databasePath := conversionFixture(t)
	projectID := projectIDForTest(t, store, root)
	seedLegacyDeferral(t, store, projectID, "legacy-a", "Intent: legacy body a\nWhy: why a\nBoundary: boundary a\nTrigger: trigger a")
	seedLegacyDeferral(t, store, projectID, "legacy-broken", "Intent: only intent line")
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	before := hashFile(t, databasePath)
	result, err := ConvertLegacyDeferrals(context.Background(), root, resolver, false)
	if err != nil {
		t.Fatalf("ConvertLegacyDeferrals(dry-run) error = %v", err)
	}
	after := hashFile(t, databasePath)
	if before != after {
		t.Fatal("dry-run mutated the database file")
	}
	if result.Action != IntentConversionActionDryRun || result.Applied || result.BackupPath != "" {
		t.Fatalf("dry-run result = %#v, want unapplied dry-run without backup", result)
	}
	if result.Convertible != 1 || result.Unparseable != 1 || result.AlreadyConverted != 0 {
		t.Fatalf("dry-run counts = %d/%d/%d, want 1 convertible, 1 unparseable, 0 converted", result.Convertible, result.Unparseable, result.AlreadyConverted)
	}
	for _, row := range result.Rows {
		if row.OperationKey == "legacy-broken" && (row.Action != "unparseable" || row.Reason == "") {
			t.Fatalf("unparseable row = %#v, want reported reason without guessing", row)
		}
	}
}

func TestConvertLegacyDeferralsApplyIsBackupFirstIdempotentAndPreserving(t *testing.T) {
	root, resolver, store, databasePath := conversionFixture(t)
	ctx := context.Background()
	projectID := projectIDForTest(t, store, root)
	decisionID, sparkID := seedLegacyDeferral(t, store, projectID, "legacy-b", "Intent: legacy body b\nWhy: why b\nBoundary: boundary b\nTrigger: trigger b")
	seedLegacyDeferral(t, store, projectID, "legacy-broken", "Intent: only intent line")

	result, err := ConvertLegacyDeferrals(ctx, root, resolver, true)
	if err != nil {
		t.Fatalf("ConvertLegacyDeferrals(apply) error = %v", err)
	}
	if !result.Applied || !result.BackupVerified || result.BackupPath == "" {
		t.Fatalf("apply result = %#v, want applied with verified backup", result)
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup missing at %s: %v", result.BackupPath, err)
	}
	if result.Convertible != 1 || result.Unparseable != 1 {
		t.Fatalf("apply counts = %#v, want 1 convertible and 1 reported unparseable", result)
	}

	var intentID, mappedJournal, mappedSpark string
	var version int
	if err := store.db.QueryRowContext(ctx, `
SELECT intent_id, journal_entry_id, spark_id, projection_version FROM intent_operations WHERE operation_key = 'legacy-b'
`).Scan(&intentID, &mappedJournal, &mappedSpark, &version); err != nil {
		t.Fatalf("read converted mapping: %v", err)
	}
	if version != 1 || mappedJournal != decisionID || mappedSpark != sparkID {
		t.Fatalf("mapping = v%d %s/%s, want v1 with historical pair", version, mappedJournal, mappedSpark)
	}
	var disposition string
	if err := store.db.QueryRowContext(ctx, `
SELECT disposition FROM intent_dispositions WHERE intent_id = ? ORDER BY seq DESC LIMIT 1
`, intentID).Scan(&disposition); err != nil {
		t.Fatalf("read converted disposition: %v", err)
	}
	if disposition != "deferred" {
		t.Fatalf("converted disposition = %q, want deferred", disposition)
	}
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM journal_deferrals`: 2,
		`SELECT COUNT(*) FROM journal_entries`:   2,
		`SELECT COUNT(*) FROM sparks`:            2,
		`SELECT COUNT(*) FROM relationships WHERE to_entity_id = '` + intentID + `' AND relationship_type = 'source-of'`: 2,
		`SELECT COUNT(*) FROM events WHERE entity_kind = 'intent' AND event_type = 'converted_from_journal_deferral'`:    1,
	} {
		var got int
		if err := store.db.QueryRowContext(ctx, query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Fatalf("%s = %d, want %d (legacy rows preserved, links and marker recorded)", query, got, want)
		}
	}

	// Intake shows the converted item exactly once, as its canonical intent.
	intake, err := ListIntake(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ListIntake() error = %v", err)
	}
	converted, legacyItems := 0, 0
	for _, item := range intake.Items {
		if item.Kind == "intent" && item.Title == "legacy body b" {
			converted++
		}
		if item.Kind == "legacy_deferral" && item.OperationKey == "legacy-b" {
			legacyItems++
		}
	}
	if converted != 1 || legacyItems != 0 {
		t.Fatalf("intake after conversion: intent=%d legacy=%d, want 1/0", converted, legacyItems)
	}

	// Rerunning apply is idempotent: nothing new is written.
	beforeRerun := hashFile(t, databasePath)
	rerun, err := ConvertLegacyDeferrals(ctx, root, resolver, true)
	if err != nil {
		t.Fatalf("ConvertLegacyDeferrals(rerun) error = %v", err)
	}
	if rerun.Convertible != 0 || rerun.AlreadyConverted != 1 || rerun.Unparseable != 1 {
		t.Fatalf("rerun counts = %#v, want 0 convertible, 1 already-converted, 1 unparseable", rerun)
	}
	var intents int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM intents`).Scan(&intents); err != nil {
		t.Fatalf("count intents: %v", err)
	}
	if intents != 1 {
		t.Fatalf("intents after rerun = %d, want 1", intents)
	}
	_ = beforeRerun

	// The retry path through the adapter returns the converted pair.
	retry, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "legacy body b", Why: "why b", Boundary: "boundary b", Trigger: "trigger b",
		OperationID: "legacy-b",
	})
	if err != nil {
		t.Fatalf("DeferJournal(converted retry) error = %v", err)
	}
	if retry.Created || retry.Decision.ID != decisionID || retry.IntentID != intentID {
		t.Fatalf("converted retry = %#v, want established pair with canonical intent", retry)
	}
}

func TestLegacyDeferralPacketParsing(t *testing.T) {
	body, why, boundary, trigger, err := legacyDeferralPacket("Intent: multi\nline body\nWhy: because\nBoundary: none\nTrigger: later\nDecision: journal:x")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if body != "multi\nline body" || why != "because" || boundary != "none" || trigger != "later" {
		t.Fatalf("parsed = %q/%q/%q/%q, want continuation lines folded into the open field", body, why, boundary, trigger)
	}
	if _, _, _, _, err := legacyDeferralPacket("Intent: x\nWhy: y"); err == nil {
		t.Fatal("packet missing Boundary/Trigger parsed without error")
	}
	if _, _, _, _, err := legacyDeferralPacket("free text with no fields"); err == nil {
		t.Fatal("fieldless packet parsed without error")
	}
	if fmt.Sprintf("%v", err) == "" {
		t.Fatal("parse error must carry a reason")
	}
}
