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

func TestDeferJournalCreatesSelfSufficientReciprocalPacket(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	options := JournalDeferOptions{Intent: "preserve context", Why: "response may be lost", Boundary: "do not execute", Trigger: "when resumed", OperationID: "op-001", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismSkill, SourceEvent: "defer", Branch: "main", Head: "abc123"}}
	result, err := store.DeferJournal(ctx, root, options)
	if err != nil {
		t.Fatalf("DeferJournal() error = %v", err)
	}
	if !result.Created || !result.InputDigestMatches || result.InputDigest == "" || result.StoredDigest != result.InputDigest {
		t.Fatalf("result = %#v, want created digest match", result)
	}
	if result.Origin == nil || result.Origin.CaptureMechanism != JournalOriginMechanismSkill || result.Origin.SourceEvent != "defer" || result.Origin.Branch != "main" || result.Origin.Head != "abc123" || !result.Origin.Supported {
		t.Fatalf("result origin = %#v, want exact supported origin", result.Origin)
	}
	packet := "Intent: preserve context\nWhy: response may be lost\nBoundary: do not execute\nTrigger: when resumed"
	if result.Decision.EntryType != "decision" || result.Decision.Message != packet+"\nSpark: "+result.Spark.ID {
		t.Fatalf("decision = %#v, want exact packet and spark link", result.Decision)
	}
	if result.Spark.Text != packet+"\nDecision: "+result.Decision.ID || result.Spark.Status != "open" || result.Spark.Alias == "" {
		t.Fatalf("spark = %#v, want exact packet and decision link", result.Spark)
	}
	if result.Decision.Scope != "defer/"+journalDeferOperationDigest(result.ProjectID, options.OperationID)[:journalDeferScopePrefixLen] || result.Spark.Scope != result.Decision.Scope {
		t.Fatalf("scopes = %q/%q, want deterministic defer scope", result.Decision.Scope, result.Spark.Scope)
	}
	var journalID, sparkID, storedDigest string
	if err := store.db.QueryRowContext(ctx, `SELECT journal_entry_id, spark_id, stored_digest FROM journal_deferrals WHERE project_id = ? AND operation_key = ?`, result.ProjectID, options.OperationID).Scan(&journalID, &sparkID, &storedDigest); err != nil {
		t.Fatalf("read deferral = %v", err)
	}
	if journalID != result.Decision.ID || sparkID != result.Spark.ID || storedDigest != result.InputDigest {
		t.Fatalf("deferral = %q/%q/%q, want reciprocal result", journalID, sparkID, storedDigest)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM events WHERE project_id = ? AND entity_kind = 'spark' AND entity_id = ?`, result.ProjectID, result.Spark.ID); got != 1 {
		t.Fatalf("spark events = %d, want 1", got)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 1 || parity.IndexRows != 1 {
		t.Fatalf("journal parity = %#v, want one ready decision", parity)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_search WHERE journal_search MATCH 'preserve'`); got != 1 {
		t.Fatalf("decision packet search hits = %d, want 1", got)
	}
}

func TestDeferJournalRetriesReturnOriginalPairAndDigestTelemetry(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	firstOptions := JournalDeferOptions{Intent: "retain nugget", Why: "retry safely", Boundary: "no autonomous execution", Trigger: "next conversation", OperationID: "op-retry"}
	first, err := store.DeferJournal(ctx, root, firstOptions)
	if err != nil {
		t.Fatalf("first DeferJournal() error = %v", err)
	}
	second, err := store.DeferJournal(ctx, root, firstOptions)
	if err != nil {
		t.Fatalf("identical retry error = %v", err)
	}
	if second.Created || !second.InputDigestMatches || second.Decision.ID != first.Decision.ID || second.Spark.ID != first.Spark.ID || second.StoredDigest != first.StoredDigest {
		t.Fatalf("identical retry = %#v, want original pair and matching digest", second)
	}
	reworded := firstOptions
	reworded.Why = "retry wording changed"
	third, err := store.DeferJournal(ctx, root, reworded)
	if err != nil {
		t.Fatalf("reworded retry error = %v", err)
	}
	if third.Created || third.InputDigestMatches || third.Decision.ID != first.Decision.ID || third.Spark.ID != first.Spark.ID || third.StoredDigest != first.StoredDigest {
		t.Fatalf("reworded retry = %#v, want original pair and mismatch telemetry", third)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_deferrals`); got != 1 {
		t.Fatalf("deferral rows after retries = %d, want 1", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("decision rows after retries = %d, want 1", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM sparks`); got != 1 {
		t.Fatalf("spark rows after retries = %d, want 1", got)
	}
}

func TestDeferJournalOriginAndRollback(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER reject_defer_origin BEFORE INSERT ON journal_origins BEGIN SELECT RAISE(ABORT, 'defer origin rejected'); END`); err != nil {
		t.Fatalf("create origin trigger: %v", err)
	}
	_, err = store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "origin rollback", Why: "trigger failure", Boundary: "none", Trigger: "now", OperationID: "op-origin",
		Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook},
	})
	if err == nil || !strings.Contains(err.Error(), "defer origin rejected") {
		t.Fatalf("DeferJournal() error = %v, want origin trigger failure", err)
	}
	assertNoDeferredRows(t, status.DatabasePath)
}

func TestDeferJournalValidationAndUnknownProject(t *testing.T) {
	cases := []JournalDeferOptions{
		{Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "op"},
		{Intent: "intent", Why: "why", Boundary: "boundary", Trigger: "trigger"},
		{Intent: strings.Repeat("x", journalDeferFieldMaxLength+1), Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "op"},
		{Intent: "intent", Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: strings.Repeat("x", journalDeferOperationMax+1)},
		{Intent: "intent", Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "bad\nkey"},
	}
	for i, options := range cases {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			resolver := PathResolver{StateHome: t.TempDir()}
			status, err := Initialize(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			store, err := OpenStore(status.DatabasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			defer store.Close()
			if _, err := store.DeferJournal(ctx, root, options); err == nil {
				t.Fatal("DeferJournal() error = nil, want validation error")
			} else {
				var validationErr *JournalDeferValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("error = %T %v, want JournalDeferValidationError", err, err)
				}
			}
			assertNoDeferredRows(t, status.DatabasePath)
		})
	}

	ctx := context.Background()
	resolver := PathResolver{StateHome: t.TempDir()}
	registered := projectRoot(t)
	if _, err := Initialize(ctx, registered, resolver); err != nil {
		t.Fatalf("Initialize(registered) error = %v", err)
	}
	unknownPath := t.TempDir()
	unknown, err := project.ResolveRoot(unknownPath)
	if err != nil {
		t.Fatalf("ResolveRoot(unknown) error = %v", err)
	}
	store, err := OpenStore(statusDatabasePath(t, registered, resolver))
	if err != nil {
		t.Fatalf("OpenStore(unknown test) error = %v", err)
	}
	defer store.Close()
	beforeProjects := rawCount(t, store.path, `SELECT COUNT(*) FROM projects`)
	if _, err := store.DeferJournal(ctx, unknown, JournalDeferOptions{Intent: "intent", Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "unknown-project"}); err == nil {
		t.Fatal("DeferJournal(unknown project) error = nil, want unregistered project error")
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM projects`); got != beforeProjects {
		t.Fatalf("projects after unknown defer = %d, want %d", got, beforeProjects)
	}
}

func TestDeferJournalHooksRollbackEveryStage(t *testing.T) {
	stages := []struct {
		name string
		set  func(*journalDeferHooks)
	}{
		{"decision", func(h *journalDeferHooks) { h.afterDecision = failDeferHook }},
		{"fts", func(h *journalDeferHooks) { h.afterFTS = failDeferHook }},
		{"spark", func(h *journalDeferHooks) { h.afterSpark = failDeferHook }},
		{"alias-event", func(h *journalDeferHooks) { h.afterAliasEvent = failDeferHook }},
		{"origin", func(h *journalDeferHooks) { h.afterOrigin = failDeferHook }},
		{"deferral", func(h *journalDeferHooks) { h.afterDeferral = failDeferHook }},
		{"before-commit", func(h *journalDeferHooks) { h.beforeCommit = failDeferHook }},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			resolver := PathResolver{StateHome: t.TempDir()}
			status, err := Initialize(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			store, err := OpenStore(status.DatabasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			defer store.Close()
			hooks := &journalDeferHooks{}
			stage.set(hooks)
			options := JournalDeferOptions{Intent: "intent", Why: "why", Boundary: "boundary", Trigger: "trigger", OperationID: "op-" + stage.name}
			if stage.name == "origin" || stage.name == "deferral" || stage.name == "before-commit" {
				dirty := false
				reconstructable := false
				options.Origin = &JournalOriginInput{
					EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismHook,
					Head: "abc123", ChangePath: "internal/state/journal_defer.go", ChangeSHA256: strings.Repeat("a", 64),
					Dirty: &dirty, Reconstructable: &reconstructable,
				}
			}
			_, err = store.deferJournalWithHooks(ctx, root, options, hooks)
			if err == nil {
				t.Fatal("deferJournalWithHooks() error = nil, want injected failure")
			}
			assertNoDeferredRows(t, status.DatabasePath)
			if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_origins`); got != 0 {
				t.Fatalf("journal origin rows after %s rollback = %d, want 0", stage.name, got)
			}
		})
	}
}

func TestDeferJournalConcurrentSameKeyConverges(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	left, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(left) error = %v", err)
	}
	defer left.Close()
	right, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(right) error = %v", err)
	}
	defer right.Close()
	options := JournalDeferOptions{Intent: "converge", Why: "response loss", Boundary: "no duplicate", Trigger: "retry", OperationID: "op-concurrent"}
	results := make([]JournalDeferResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); results[0], errs[0] = left.DeferJournal(ctx, root, options) }()
	go func() { defer wg.Done(); results[1], errs[1] = right.DeferJournal(ctx, root, options) }()
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent defer %d error = %v", i, err)
		}
	}
	if results[0].Decision.ID != results[1].Decision.ID || results[0].Spark.ID != results[1].Spark.ID {
		t.Fatalf("concurrent results = %#v, want one pair", results)
	}
	if results[0].Created == results[1].Created {
		t.Fatalf("concurrent Created flags = %t/%t, want exactly one creator", results[0].Created, results[1].Created)
	}
	assertDeferredCounts(t, status.DatabasePath, 1, 1, 1, 1, 1, 1)
}

func TestDeferJournalConcurrentDifferentKeysBothCommit(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	stores := make([]*Store, 2)
	for i := range stores {
		stores[i], err = OpenStore(status.DatabasePath)
		if err != nil {
			t.Fatalf("OpenStore(%d) error = %v", i, err)
		}
		defer stores[i].Close()
	}
	results := make([]JournalDeferResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range stores {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = stores[i].DeferJournal(ctx, root, JournalDeferOptions{
				Intent: "independent intent", Why: "independent reason", Boundary: "independent boundary", Trigger: "independent trigger", OperationID: fmt.Sprintf("op-independent-%d", i),
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent defer %d error = %v", i, err)
		}
		if !results[i].Created {
			t.Fatalf("concurrent defer %d Created = false, want true", i)
		}
	}
	if results[0].Decision.ID == results[1].Decision.ID || results[0].Spark.ID == results[1].Spark.ID {
		t.Fatalf("different operation IDs collided: %#v", results)
	}
	assertDeferredCounts(t, status.DatabasePath, 2, 2, 2, 2, 2, 2)
}

func TestDeferJournalSameOperationIDAcrossProjectsIsIndependent(t *testing.T) {
	ctx := context.Background()
	resolver := PathResolver{StateHome: t.TempDir()}
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	if _, err := Initialize(ctx, rootA, resolver); err != nil {
		t.Fatalf("Initialize(project A) error = %v", err)
	}
	if _, err := Initialize(ctx, rootB, resolver); err != nil {
		t.Fatalf("Initialize(project B) error = %v", err)
	}
	path := statusDatabasePath(t, rootA, resolver)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	options := JournalDeferOptions{Intent: "same key", Why: "project scoped", Boundary: "no cross lookup", Trigger: "retry", OperationID: "same-operation"}
	resultA, err := store.DeferJournal(ctx, rootA, options)
	if err != nil {
		t.Fatalf("DeferJournal(project A) error = %v", err)
	}
	resultB, err := store.DeferJournal(ctx, rootB, options)
	if err != nil {
		t.Fatalf("DeferJournal(project B) error = %v", err)
	}
	if !resultA.Created || !resultB.Created || resultA.ProjectID == resultB.ProjectID {
		t.Fatalf("project results = %#v/%#v, want two created project-scoped rows", resultA, resultB)
	}
	if resultA.Decision.ID == resultB.Decision.ID || resultA.Spark.ID == resultB.Spark.ID || resultA.Decision.Scope == resultB.Decision.Scope {
		t.Fatalf("cross-project deterministic identities collided: %#v/%#v", resultA, resultB)
	}
	retryA, err := store.DeferJournal(ctx, rootA, options)
	if err != nil {
		t.Fatalf("retry project A error = %v", err)
	}
	retryB, err := store.DeferJournal(ctx, rootB, options)
	if err != nil {
		t.Fatalf("retry project B error = %v", err)
	}
	if retryA.Created || retryB.Created || !retryA.InputDigestMatches || !retryB.InputDigestMatches || retryA.Decision.ID != resultA.Decision.ID || retryB.Decision.ID != resultB.Decision.ID || retryA.Spark.ID != resultA.Spark.ID || retryB.Spark.ID != resultB.Spark.ID {
		t.Fatalf("cross-project retries = %#v/%#v, want each original pair", retryA, retryB)
	}
	if retryA.StoredDigest != resultA.StoredDigest || retryB.StoredDigest != resultB.StoredDigest {
		t.Fatalf("cross-project retry digests = %q/%q, want original stored digests", retryA.StoredDigest, retryB.StoredDigest)
	}
	assertDeferredCounts(t, path, 2, 2, 2, 2, 2, 2)
	if got := rawCount(t, path, `SELECT COUNT(*) FROM journal_deferrals WHERE operation_key = 'same-operation'`); got != 2 {
		t.Fatalf("same operation rows across projects = %d, want 2", got)
	}
}

func failDeferHook(*sql.Tx) error { return errors.New("injected defer failure") }

func assertNoDeferredRows(t *testing.T, databasePath string) {
	t.Helper()
	assertDeferredCounts(t, databasePath, 0, 0, 0, 0, 0, 0)
	if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM journal_origins`); got != 0 {
		t.Fatalf("journal origin rows = %d, want 0", got)
	}
}

func assertDeferredCounts(t *testing.T, databasePath string, journal, search, sparks, aliases, events, deferrals int) {
	t.Helper()
	for table, want := range map[string]int{"journal_entries": journal, "journal_search": search, "sparks": sparks, "events": events, "journal_deferrals": deferrals} {
		if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM `+table); got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
	if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM aliases WHERE namespace = 'spark'`); got != aliases {
		t.Fatalf("spark alias rows = %d, want %d", got, aliases)
	}
	// The adapter can no longer create a legacy-only deferral: every legacy
	// pair must be backed by one canonical Intent, one immutable deferral
	// payload, and one version-1 operation mapping carrying the pair IDs.
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM intents`:                                          deferrals,
		`SELECT COUNT(*) FROM intent_deferrals`:                                 deferrals,
		`SELECT COUNT(*) FROM intent_operations WHERE projection_version = 1`:   deferrals,
		`SELECT COUNT(*) FROM intent_operations WHERE journal_entry_id IS NULL`: 0,
		`SELECT COUNT(*) FROM aliases WHERE namespace = 'intent'`:               deferrals,
	} {
		if got := rawCount(t, databasePath, query); got != want {
			t.Fatalf("%s = %d, want %d", query, got, want)
		}
	}
}

func statusDatabasePath(t *testing.T, root project.Root, resolver PathResolver) string {
	t.Helper()
	path, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	return path
}
