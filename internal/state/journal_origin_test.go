package state

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestLogJournalOriginRoundTripsAndPreservesSearchParity(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	dirty := false
	reconstructable := false
	logged, err := store.LogJournal(ctx, root, JournalLogOptions{
		Entry:            "decision(origin): retain provenance separately",
		ObservedBranch:   "feat/origin",
		ObservedWorktree: "/worktree",
		HarnessSessionID: "harness-session",
		Origin: &JournalOriginInput{
			EnvelopeVersion:        JournalOriginEnvelopeVersion,
			CaptureMechanism:       "vendor-tool",
			ObservedHarness:        "codex",
			ObservedHarnessVersion: "1.2.3",
			HarnessSessionID:       "origin-session",
			AgentID:                "agent-1",
			SourceEvent:            "post-tool",
			Branch:                 "feat/origin",
			Worktree:               "/worktree",
			Head:                   "abc123",
			ChangePath:             "internal/state/journal.go",
			ChangeSHA256:           strings.Repeat("A", 64),
			Dirty:                  &dirty,
			Reconstructable:        &reconstructable,
			DurableResultKind:      "change",
			DurableResultID:        "change-001",
		},
	})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	show, err := store.ShowJournal(ctx, root, logged.ID)
	if err != nil {
		t.Fatalf("ShowJournal() error = %v", err)
	}
	if show.Origin == nil {
		t.Fatal("ShowJournal().Origin = nil, want origin")
	}
	origin := show.Origin
	if origin.JournalEntryID != logged.ID || origin.ProjectID != logged.ProjectID || origin.EnvelopeVersion != 1 || origin.CaptureMechanism != "vendor-tool" || !origin.Supported {
		t.Fatalf("origin identity = %#v, want canonical project/id, v1, vendor-tool, supported", origin)
	}
	if origin.ObservedHarness != "codex" || origin.ObservedHarnessVersion != "1.2.3" || origin.HarnessSessionID != "origin-session" || origin.AgentID != "agent-1" || origin.SourceEvent != "post-tool" || origin.Branch != "feat/origin" || origin.Worktree != "/worktree" || origin.Head != "abc123" || origin.ChangePath != "internal/state/journal.go" || origin.ChangeSHA256 != strings.Repeat("a", 64) || origin.DurableResultKind != "change" || origin.DurableResultID != "change-001" {
		t.Fatalf("origin fields = %#v, want normalized round-trip", origin)
	}
	if origin.Dirty == nil || *origin.Dirty {
		t.Fatalf("origin dirty = %#v, want pointer to false", origin.Dirty)
	}
	if origin.Reconstructable == nil || *origin.Reconstructable {
		t.Fatalf("origin reconstructable = %#v, want pointer to false", origin.Reconstructable)
	}
	var canonicalCreatedAt string
	if err := store.db.QueryRowContext(ctx, `SELECT created_at FROM journal_entries WHERE id = ? AND project_id = ?`, logged.ID, logged.ProjectID).Scan(&canonicalCreatedAt); err != nil {
		t.Fatalf("read canonical created_at: %v", err)
	}
	if origin.CreatedAt != canonicalCreatedAt {
		t.Fatalf("origin created_at = %q, want canonical %q", origin.CreatedAt, canonicalCreatedAt)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready || parity.Changed != 0 {
		t.Fatalf("search parity = %#v, want ready and unchanged", parity)
	}
	encoded, err := json.Marshal(show)
	if err != nil {
		t.Fatalf("marshal show = %v", err)
	}
	if !strings.Contains(string(encoded), `"origin"`) || !strings.Contains(string(encoded), `"supported":true`) || !strings.Contains(string(encoded), `"dirty":false`) || !strings.Contains(string(encoded), `"reconstructable":false`) {
		t.Fatalf("show JSON = %s, want additive origin and nullable booleans", encoded)
	}
}

func TestLogJournalWithoutOriginAndUnknownEnvelopeShowAdditively(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	plain, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "note(no-origin): plain entry"})
	if err != nil {
		t.Fatalf("plain LogJournal() error = %v", err)
	}
	plainShow, err := store.ShowJournal(ctx, root, plain.ID)
	if err != nil {
		t.Fatalf("plain ShowJournal() error = %v", err)
	}
	if plainShow.Origin != nil {
		t.Fatalf("plain ShowJournal().Origin = %#v, want nil", plainShow.Origin)
	}
	encoded, err := json.Marshal(plainShow)
	if err != nil {
		t.Fatalf("marshal plain show = %v", err)
	}
	if strings.Contains(string(encoded), `"origin"`) {
		t.Fatalf("plain show JSON = %s, origin must be omitted", encoded)
	}

	unknown, err := store.LogJournal(ctx, root, JournalLogOptions{
		Entry: "discover(unknown): future envelope",
		Origin: &JournalOriginInput{
			EnvelopeVersion:  2,
			CaptureMechanism: "future-harness",
			SourceEvent:      "future-event",
		},
	})
	if err != nil {
		t.Fatalf("unknown envelope LogJournal() error = %v", err)
	}
	unknownShow, err := store.ShowJournal(ctx, root, unknown.ID)
	if err != nil {
		t.Fatalf("unknown envelope ShowJournal() error = %v", err)
	}
	if unknownShow.Origin == nil || unknownShow.Origin.EnvelopeVersion != 2 || unknownShow.Origin.CaptureMechanism != "future-harness" || unknownShow.Origin.SourceEvent != "future-event" || unknownShow.Origin.Supported {
		t.Fatalf("unknown envelope origin = %#v, want stored fields and supported=false", unknownShow.Origin)
	}
	if unknownShow.Origin.Dirty != nil || unknownShow.Origin.Reconstructable != nil {
		t.Fatalf("unknown envelope booleans = dirty:%#v reconstructable:%#v, want both nil", unknownShow.Origin.Dirty, unknownShow.Origin.Reconstructable)
	}
}

func TestLogJournalOriginValidationRollsBackNothing(t *testing.T) {
	boolTrue := true
	cases := []struct {
		name   string
		origin JournalOriginInput
	}{
		{"version", JournalOriginInput{EnvelopeVersion: 0, CaptureMechanism: "manual"}},
		{"mechanism", JournalOriginInput{EnvelopeVersion: 1}},
		{"mechanism length", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: strings.Repeat("x", journalOriginMechanismMaxLength+1)}},
		{"field length", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", AgentID: strings.Repeat("x", journalOriginStringMaxLength+1)}},
		{"absolute path", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "/tmp/change"}},
		{"traversal path", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "../change"}},
		{"path only", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "internal/state/journal.go"}},
		{"digest only", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangeSHA256: strings.Repeat("a", 64)}},
		{"dirty only", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", Dirty: boolPtr(true)}},
		{"reconstructable only", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", Reconstructable: boolPtr(true)}},
		{"reconstructable without head", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "internal/state/journal.go", ChangeSHA256: strings.Repeat("a", 64), Dirty: boolPtr(false), Reconstructable: boolPtr(true)}},
		{"dirty without reconstructable", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "internal/state/journal.go", ChangeSHA256: strings.Repeat("a", 64), Dirty: boolPtr(true)}},
		{"digest", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangeSHA256: "not-a-digest"}},
		{"durable result", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", DurableResultKind: "change"}},
		{"conflicting booleans", JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "internal/state/journal.go", ChangeSHA256: strings.Repeat("a", 64), Dirty: &boolTrue, Reconstructable: &boolTrue}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			stateHome := t.TempDir()
			resolver := PathResolver{StateHome: stateHome}
			status, err := Initialize(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			store, err := OpenStore(status.DatabasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			defer store.Close()
			if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "note(invalid): reject", Origin: &tc.origin}); err == nil {
				t.Fatal("LogJournal() error = nil, want origin validation error")
			}
			for table := range map[string]bool{"journal_entries": true, "journal_search": true, "journal_origins": true} {
				var count int
				if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				if count != 0 {
					t.Fatalf("%s rows = %d, want 0", table, count)
				}
			}
		})
	}
}

func TestNormalizeJournalOriginInputAcceptsChangeEvidenceVariants(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	clean := JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismSkill, Head: "abc123", ChangePath: "docs/change.md", ChangeSHA256: strings.Repeat("a", 64), Dirty: boolPtr(false), Reconstructable: boolPtr(true)}
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(clean): reconstructable", Origin: &clean}); err != nil {
		t.Fatalf("clean reconstructable LogJournal() error = %v", err)
	}
	dirty := JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook, ChangePath: "docs/change.md", ChangeSHA256: strings.Repeat("b", 64), Dirty: boolPtr(true), Reconstructable: boolPtr(false)}
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(dirty): unreconstructable", Origin: &dirty}); err != nil {
		t.Fatalf("dirty unreconstructable LogJournal() error = %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestLogJournalOriginInsertFailureRollsBackCanonicalAndSearch(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `
CREATE TRIGGER reject_journal_origins BEFORE INSERT ON journal_origins
BEGIN SELECT RAISE(ABORT, 'origin insert rejected'); END`); err != nil {
		t.Fatalf("create origin trigger: %v", err)
	}
	_, err = store.LogJournal(ctx, root, JournalLogOptions{
		Entry:  "note(trigger): rollback",
		Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook},
	})
	if err == nil || !strings.Contains(err.Error(), "origin insert rejected") {
		t.Fatalf("LogJournal() error = %v, want trigger error", err)
	}
	for table := range map[string]bool{"journal_entries": true, "journal_search": true, "journal_origins": true} {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0 after rollback", table, count)
		}
	}
}

func TestShowJournalBackfilledOriginReportsOnlyObservableFields(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
	defer store.Close()
	insertMigrationJournalRow(t, store.db, projectID, "journal-backfilled-origin", "decision", "migration", "backfilled", "feat/backfilled", "/worktree", "harness-backfilled")
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	show, err := store.ShowJournal(ctx, root, "journal-backfilled-origin")
	if err != nil {
		t.Fatalf("ShowJournal() error = %v", err)
	}
	if show.Origin == nil || !show.Origin.Supported || show.Origin.CaptureMechanism != JournalOriginMechanismUnknown || show.Origin.Branch != "feat/backfilled" || show.Origin.Worktree != "/worktree" || show.Origin.HarnessSessionID != "harness-backfilled" {
		t.Fatalf("backfilled origin = %#v, want honest unknown and observable fields", show.Origin)
	}
	if show.Origin.ObservedHarness != "" || show.Origin.AgentID != "" || show.Origin.SourceEvent != "" || show.Origin.Head != "" || show.Origin.ChangePath != "" || show.Origin.DurableResultID != "" {
		t.Fatalf("backfilled origin inferred unobservable values: %#v", show.Origin)
	}
}

func TestShowJournalProjectScopeCannotBeSpoofedByOriginInput(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	logged, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "note(scope): canonical identity", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismManual}})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	show, err := store.ShowJournal(ctx, root, logged.ID)
	if err != nil {
		t.Fatalf("ShowJournal() error = %v", err)
	}
	if show.Origin == nil || show.Origin.ProjectID != logged.ProjectID || show.Origin.JournalEntryID != logged.ID {
		t.Fatalf("origin identity = %#v, want logged canonical project/id", show.Origin)
	}
}

func TestNormalizeJournalOriginInputRejectsInvalidPathNUL(t *testing.T) {
	_, err := normalizeJournalOriginInput(JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual", ChangePath: "file\x00.txt"})
	if err == nil {
		t.Fatal("normalizeJournalOriginInput() error = nil, want NUL rejection")
	}
}
