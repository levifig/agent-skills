package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestJournalContextLayersWrapBranchEntriesAndOpenTasks(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Seed two open tasks and one done task; only the open ones should surface.
	openTask, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open task"})
	if err != nil {
		t.Fatalf("CreateTask(open) error = %v", err)
	}
	doneTask, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Done task"})
	if err != nil {
		t.Fatalf("CreateTask(done) error = %v", err)
	}
	if _, err := UpdateTaskStatus(context.Background(), root, resolver, doneTask.Task.Alias, "done"); err != nil {
		t.Fatalf("UpdateTaskStatus(done) error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	// Project wrap plus branch entries on the current branch.
	seedJournalEntry(t, store, projectID, "wrap", "project", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "feat", "chose approach A", "feat/context", "2026-07-01T10:00:00Z")
	seedJournalEntry(t, store, projectID, "discover", "other", "unrelated branch note", "feat/other", "2026-07-01T11:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/context"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.ContractVersion != JournalContextContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, JournalContextContractVersion)
	}
	assertSessionProjectContext(t, root, StateJSONContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	if result.LatestWrap == nil || result.LatestWrap.Message != "project checkpoint" {
		t.Fatalf("latest wrap = %#v, want project checkpoint", result.LatestWrap)
	}
	if len(result.BranchEntries) != 1 || result.BranchEntries[0].Message != "chose approach A" {
		t.Fatalf("branch entries = %#v, want single feat/context entry", result.BranchEntries)
	}
	if len(result.OpenTasks) != 1 || result.OpenTasks[0].Ref != openTask.Task.Alias {
		t.Fatalf("open tasks = %#v, want single open task %s", result.OpenTasks, openTask.Task.Alias)
	}
	if result.OpenTasks[0].Title != "Open task" {
		t.Fatalf("open task title = %q, want Open task", result.OpenTasks[0].Title)
	}
}

func TestJournalContextFreshBranchDegradesToProjectWrapAndTasks(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Still open"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "wrap", "project", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "main", "main branch work", "main", "2026-07-01T10:00:00Z")

	// A brand-new branch with no entries of its own: wrap + open tasks survive,
	// branch layer is empty.
	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/fresh"})
	if err != nil {
		t.Fatalf("JournalContext(fresh branch) error = %v", err)
	}
	if result.LatestWrap == nil || result.LatestWrap.Message != "project checkpoint" {
		t.Fatalf("latest wrap = %#v, want project checkpoint", result.LatestWrap)
	}
	if len(result.BranchEntries) != 0 {
		t.Fatalf("fresh-branch entries = %#v, want none", result.BranchEntries)
	}
	if len(result.OpenTasks) != 1 {
		t.Fatalf("open tasks = %#v, want the single open task", result.OpenTasks)
	}
}

func TestJournalContextNoWrapDegradesGracefully(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "decision", "feat", "branch-only work", "feat/context", "2026-07-01T10:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{Branch: "feat/context"})
	if err != nil {
		t.Fatalf("JournalContext(no wrap) error = %v", err)
	}
	if result.LatestWrap != nil {
		t.Fatalf("latest wrap = %#v, want nil (no wrap exists)", result.LatestWrap)
	}
	if len(result.BranchEntries) != 1 {
		t.Fatalf("branch entries = %#v, want single entry", result.BranchEntries)
	}
	if len(result.OpenTasks) != 0 {
		t.Fatalf("open tasks = %#v, want none", result.OpenTasks)
	}
}

func TestJournalContextNoBranchOmitsBranchLayer(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "wrap", "project", "project checkpoint", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "main", "main branch work", "main", "2026-07-01T10:00:00Z")

	result, err := store.JournalContext(context.Background(), root, JournalContextOptions{})
	if err != nil {
		t.Fatalf("JournalContext(no branch) error = %v", err)
	}
	if result.LatestWrap == nil {
		t.Fatal("latest wrap = nil, want project checkpoint")
	}
	if len(result.BranchEntries) != 0 {
		t.Fatalf("branch entries = %#v, want none when no branch given", result.BranchEntries)
	}
	assertJournalContextSourceAvailabilityJSON(t, result.BranchRecency, false)
}

func TestJournalContextV2ActiveTruthSurvivesNoisyBranchesAndPaginatesExactly(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	for _, title := range []string{"Task one", "Task two", "Task three"} {
		if _, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: title}); err != nil {
			t.Fatalf("CreateTask(%q) error = %v", title, err)
		}
	}

	store := openTestStore(t, root, stateHome)
	dirty := true
	reconstructable := false
	deferred, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "retain deferred state", Why: "it must survive recency noise", Boundary: "do not execute now", Trigger: "when shaping resumes", OperationID: "context-v2-defer",
		Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismSkill, Branch: "feat/context", ChangePath: "docs/changes/context/change.md", ChangeSHA256: strings.Repeat("a", 64), Dirty: &dirty, Reconstructable: &reconstructable},
	})
	if err != nil {
		store.Close()
		t.Fatalf("DeferJournal() error = %v", err)
	}
	projectID := projectIDForTest(t, store, root)
	projectWrapID := seedJournalEntry(t, store, projectID, "wrap", "project", "authoritative synthesis", "main", "2026-07-01T08:00:00Z")
	seedJournalEntry(t, store, projectID, "wrap", "feature", "newer but scoped checkpoint", "feat/context", "2026-07-01T23:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "lineage/reliability", "older lineage", "main", "2026-07-01T08:30:00Z")
	lineageID := seedJournalEntry(t, store, projectID, "decision", "lineage/reliability", "current lineage", "feat/context", "2026-07-01T09:30:00Z")
	seedJournalEntry(t, store, projectID, "block", "database-lock", "first block", "feat/context", "2026-07-01T10:00:00Z")
	seedJournalEntry(t, store, projectID, "unblock", "database-lock", "resolved", "feat/context", "2026-07-01T10:30:00Z")
	reopenedID := seedJournalEntry(t, store, projectID, "block", "database-lock", "regressed and reopened", "feat/context", "2026-07-01T11:00:00Z")
	seedJournalEntry(t, store, projectID, "block", "resolved-key", "temporary", "other", "2026-07-01T11:10:00Z")
	seedJournalEntry(t, store, projectID, "unblock", "resolved-key", "fixed", "other", "2026-07-01T11:20:00Z")
	seedJournalEntry(t, store, projectID, "unblock", "never-blocked", "cannot match", "third", "2026-07-01T11:30:00Z")
	for i := 0; i < 36; i++ {
		branch := []string{"feat/context", "other", "third"}[i%3]
		seedJournalEntry(t, store, projectID, "discover", "noise", fmt.Sprintf("noise-%02d", i), branch, fmt.Sprintf("2026-07-02T%02d:%02d:00Z", i/3, i%3))
	}
	// Context is canonical-table-only: deleting an FTS row must not affect it.
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE rowid = (SELECT MIN(rowid) FROM journal_search)`); err != nil {
		store.Close()
		t.Fatalf("diverge journal_search: %v", err)
	}
	beforeBytes, err := os.ReadFile(status.DatabasePath)
	if err != nil {
		store.Close()
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	beforeProjects := countRows(t, store, `SELECT COUNT(*) FROM projects`)
	beforePaths := countRows(t, store, `SELECT COUNT(*) FROM project_paths`)

	first, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "feat/context", LineageKeys: []string{"reliability"}, BranchLimit: 3, TaskLimit: 1})
	if err != nil {
		store.Close()
		t.Fatalf("JournalContext() error = %v", err)
	}
	if first.ContractVersion != 2 || first.ProjectSynthesis.AvailableCount != 1 || first.ProjectSynthesis.Items[0].ID != projectWrapID {
		t.Fatalf("project synthesis = %#v", first.ProjectSynthesis)
	}
	if first.LatestCheckpoint.AvailableCount != 0 || len(first.LatestCheckpoint.Items) != 0 {
		t.Fatalf("latest checkpoint = %#v, want absent while project synthesis exists", first.LatestCheckpoint)
	}
	if first.ActiveLineage.AvailableCount != 1 || first.ActiveLineage.Items[0].ID != lineageID {
		t.Fatalf("active lineage = %#v", first.ActiveLineage)
	}
	if first.UnresolvedBlockers.AvailableCount != 1 || first.UnresolvedBlockers.Items[0].Block.ID != reopenedID || first.UnresolvedBlockers.Items[0].PreviousUnblockID == "" {
		t.Fatalf("unresolved blockers = %#v", first.UnresolvedBlockers)
	}
	if len(first.Diagnostics) != 1 || first.Diagnostics[0].Code != "journal-context-unmatched-unblock" {
		t.Fatalf("diagnostics = %#v", first.Diagnostics)
	}
	if first.DeferredIntent.AvailableCount != 1 || first.DeferredIntent.Items[0].Decision.ID != deferred.Decision.ID || first.DeferredIntent.Items[0].Spark.ID != deferred.Spark.ID || first.DeferredIntent.Items[0].Origin == nil || first.DeferredIntent.Items[0].Origin.ChangePath == "" {
		t.Fatalf("deferred intent = %#v", first.DeferredIntent)
	}
	if first.TransitionalTasks.AvailableCount != 3 || first.TransitionalTasks.ShownCount != 1 || !first.TransitionalTasks.Truncated || first.TransitionalTasks.Cursor == "" {
		t.Fatalf("tasks = %#v", first.TransitionalTasks)
	}
	for _, entry := range first.BranchRecency.Items {
		if entry.ID == projectWrapID || entry.ID == lineageID || entry.ID == reopenedID {
			t.Fatalf("branch recency retained active ID %s", entry.ID)
		}
	}

	seen := map[string]struct{}{}
	page := first
	for {
		for _, entry := range page.BranchRecency.Items {
			if _, duplicate := seen[entry.ID]; duplicate {
				t.Fatalf("duplicate branch page entry %s", entry.ID)
			}
			seen[entry.ID] = struct{}{}
		}
		if page.BranchRecency.Cursor == "" {
			break
		}
		page, err = store.JournalContext(ctx, root, JournalContextOptions{Branch: "feat/context", LineageKeys: []string{"reliability"}, BranchLimit: 3, Cursor: page.BranchRecency.Cursor, CursorLayer: JournalContextLayerBranch})
		if err != nil {
			store.Close()
			t.Fatalf("JournalContext(next branch page) error = %v", err)
		}
	}
	if len(seen) != first.BranchRecency.AvailableCount {
		t.Fatalf("branch page union = %d, want %d", len(seen), first.BranchRecency.AvailableCount)
	}
	if strings.TrimSpace(first.BranchRecency.ExpandCommand) == "" || first.BranchRecency.Items == nil || first.ActiveLineage.Items == nil || first.UnresolvedBlockers.Items == nil || first.DeferredIntent.Items == nil || first.TransitionalTasks.Items == nil {
		t.Fatal("layer metadata/items are incomplete")
	}
	for _, layer := range []any{first.ProjectSynthesis, first.LatestCheckpoint, first.ActiveLineage, first.UnresolvedBlockers, first.DeferredIntent, first.BranchRecency, first.TransitionalTasks} {
		assertJournalContextSourceAvailabilityJSON(t, layer, true)
	}

	afterBytes, err := os.ReadFile(status.DatabasePath)
	if err != nil {
		store.Close()
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if string(beforeBytes) != string(afterBytes) {
		store.Close()
		t.Fatal("journal context mutated database bytes")
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM projects`); got != beforeProjects {
		store.Close()
		t.Fatalf("projects = %d, want %d", got, beforeProjects)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM project_paths`); got != beforePaths {
		store.Close()
		t.Fatalf("project_paths = %d, want %d", got, beforePaths)
	}
	store.Close()
}

func TestJournalContextV2ScopedCheckpointFallbackAndCursorErrors(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	for _, title := range []string{"First", "Second"} {
		if _, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: title}); err != nil {
			t.Fatalf("CreateTask() error = %v", err)
		}
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "wrap", "older", "old scoped", "main", "2026-07-01T08:00:00Z")
	latestID := seedJournalEntry(t, store, projectID, "wrap", "feature", "latest scoped", "main", "2026-07-01T09:00:00Z")
	var deferred []JournalDeferResult
	for i := 0; i < 2; i++ {
		item, err := store.DeferJournal(ctx, root, JournalDeferOptions{Intent: fmt.Sprintf("deferred %d", i), Why: "test active truth", Boundary: "wait", Trigger: "later", OperationID: fmt.Sprintf("cursor-deferral-%d", i)})
		if err != nil {
			store.Close()
			t.Fatalf("DeferJournal(%d) error = %v", i, err)
		}
		deferred = append(deferred, item)
	}
	if _, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "ordinary spark", Scope: "ordinary"}); err != nil {
		store.Close()
		t.Fatalf("CaptureSpark(ordinary) error = %v", err)
	}

	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main", TaskLimit: 1, DeferralLimit: 1})
	if err != nil {
		store.Close()
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.ProjectSynthesis.AvailableCount != 0 || result.LatestCheckpoint.AvailableCount != 1 || result.LatestCheckpoint.Items[0].Entry.ID != latestID || result.LatestCheckpoint.Items[0].ProjectSynthesis || !strings.Contains(result.LatestCheckpoint.Items[0].Label, "not project synthesis") {
		t.Fatalf("checkpoint fallback = %#v", result.LatestCheckpoint)
	}
	if result.DeferredIntent.AvailableCount != 2 || !result.DeferredIntent.Truncated || result.DeferredIntent.Cursor == "" {
		t.Fatalf("deferred intent = %#v, want only two journal deferrals and no ordinary spark", result.DeferredIntent)
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Cursor: "not-base64"}); err == nil {
		store.Close()
		t.Fatal("malformed cursor succeeded")
	} else {
		var invalid *JournalContextCursorInvalidError
		if !errors.As(err, &invalid) || invalid.Code != JournalContextCursorInvalidCode {
			store.Close()
			t.Fatalf("malformed cursor error = %T %v", err, err)
		}
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "other", TaskLimit: 1, Cursor: result.TransitionalTasks.Cursor, CursorLayer: JournalContextLayerTasks}); err == nil {
		store.Close()
		t.Fatal("wrong-branch cursor succeeded")
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main", BranchLimit: 1, Cursor: result.TransitionalTasks.Cursor, CursorLayer: JournalContextLayerBranch}); err == nil {
		store.Close()
		t.Fatal("wrong-layer cursor succeeded")
	}
	otherRoot := projectRoot(t)
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		store.Close()
		t.Fatalf("Initialize(other project) error = %v", err)
	}
	otherStore := openTestStore(t, otherRoot, stateHome)
	if _, err := otherStore.JournalContext(ctx, otherRoot, JournalContextOptions{Branch: "main", TaskLimit: 1, Cursor: result.TransitionalTasks.Cursor, CursorLayer: JournalContextLayerTasks}); err == nil {
		otherStore.Close()
		store.Close()
		t.Fatal("wrong-project cursor succeeded")
	}
	otherStore.Close()
	// Disposition is canonical: resolving the Intent (not marking the legacy
	// spark done) is what removes a deferred item from active truth.
	if _, err := store.appendIntentDisposition(ctx, root, IntentDispositionOptions{IntentRef: deferred[0].IntentAlias, Reason: "resolved in test"}, "resolved"); err != nil {
		store.Close()
		t.Fatalf("resolve deferred intent: %v", err)
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main", DeferralLimit: 1, Cursor: result.DeferredIntent.Cursor, CursorLayer: JournalContextLayerDeferrals}); err == nil {
		store.Close()
		t.Fatal("stale deferral cursor succeeded")
	} else {
		var stale *JournalContextCursorStaleError
		if !errors.As(err, &stale) {
			store.Close()
			t.Fatalf("stale deferral cursor error = %T %v", err, err)
		}
	}
	refreshed, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main"})
	if err != nil {
		store.Close()
		t.Fatalf("JournalContext(refreshed) error = %v", err)
	}
	if refreshed.DeferredIntent.AvailableCount != 1 {
		store.Close()
		t.Fatalf("resolved deferred intent count = %d, want 1", refreshed.DeferredIntent.AvailableCount)
	}
	if _, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: "Third"}); err != nil {
		store.Close()
		t.Fatalf("CreateTask(stale) error = %v", err)
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main", TaskLimit: 1, Cursor: result.TransitionalTasks.Cursor, CursorLayer: JournalContextLayerTasks}); err == nil {
		store.Close()
		t.Fatal("stale task cursor succeeded")
	} else {
		var stale *JournalContextCursorStaleError
		if !errors.As(err, &stale) || stale.Code != JournalContextCursorStaleCode || stale.RestartCommand == "" {
			store.Close()
			t.Fatalf("stale cursor error = %T %v", err, err)
		}
	}
	store.Close()
}

func TestJournalContextV2LineageCursorBindsCanonicalKeyBatchAndReplayCommand(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "decision", "lineage/foo", "foo", "feature/quoted branch", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "decision", "lineage/bar", "bar", "feature/quoted branch", "2026-07-01T10:00:00Z")

	options := JournalContextOptions{Branch: "feature/quoted branch", LineageKeys: []string{"foo", "lineage/bar", "foo"}, LineageLimit: 1}
	first, err := store.JournalContext(ctx, root, options)
	if err != nil {
		t.Fatalf("JournalContext(first lineage page) error = %v", err)
	}
	if first.ActiveLineage.AvailableCount != 2 || first.ActiveLineage.Cursor == "" {
		t.Fatalf("active lineage = %#v, want two canonical keys and cursor", first.ActiveLineage)
	}
	wantCommand := "loaf journal context --layer active-lineage --limit 1 --branch 'feature/quoted branch' --cursor " + posixShellQuote(first.ActiveLineage.Cursor)
	if first.ActiveLineage.ExpandCommand != wantCommand {
		t.Fatalf("ExpandCommand = %q, want %q", first.ActiveLineage.ExpandCommand, wantCommand)
	}
	second, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: options.Branch, LineageKeys: []string{"bar", "foo"}, LineageLimit: 1, Cursor: first.ActiveLineage.Cursor, CursorLayer: JournalContextLayerLineage})
	if err != nil {
		t.Fatalf("JournalContext(replayed lineage command options) error = %v", err)
	}
	if second.ActiveLineage.ShownCount != 1 || second.ActiveLineage.Cursor != "" || second.ActiveLineage.Items[0].ID == first.ActiveLineage.Items[0].ID {
		t.Fatalf("second lineage page = %#v", second.ActiveLineage)
	}
	if _, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: options.Branch, LineageKeys: []string{"foo"}, LineageLimit: 1, Cursor: first.ActiveLineage.Cursor, CursorLayer: JournalContextLayerLineage}); err == nil {
		t.Fatal("changed lineage-key batch cursor succeeded")
	} else {
		var stale *JournalContextCursorStaleError
		if !errors.As(err, &stale) {
			t.Fatalf("changed lineage-key batch error = %T %v", err, err)
		}
	}
}

func TestJournalContextV2LineageRequiresExactRetainedKeysAndDeduplicatesOnlyMatches(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	fooID := seedJournalEntry(t, store, projectID, "decision", "lineage/foo", "foo retained lineage", "feature/exact", "2026-07-01T09:00:00Z")
	fooExtraID := seedJournalEntry(t, store, projectID, "decision", "lineage/foo-extra", "different lineage", "feature/exact", "2026-07-01T10:00:00Z")

	withoutKeys, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "feature/exact"})
	if err != nil {
		t.Fatalf("JournalContext(no lineage keys) error = %v", err)
	}
	if withoutKeys.ActiveLineage.AvailableCount != 0 || len(withoutKeys.ActiveLineage.Items) != 0 {
		t.Fatalf("active lineage without retained keys = %#v, want empty", withoutKeys.ActiveLineage)
	}
	assertJournalContextEntryIDs(t, withoutKeys.BranchRecency.Items, map[string]bool{fooID: true, fooExtraID: true})

	withFoo, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "feature/exact", LineageKeys: []string{"foo"}})
	if err != nil {
		t.Fatalf("JournalContext(exact foo key) error = %v", err)
	}
	if withFoo.ActiveLineage.AvailableCount != 1 || len(withFoo.ActiveLineage.Items) != 1 || withFoo.ActiveLineage.Items[0].ID != fooID {
		t.Fatalf("active lineage for foo = %#v, want exact foo only", withFoo.ActiveLineage)
	}
	assertJournalContextEntryIDs(t, withFoo.BranchRecency.Items, map[string]bool{fooID: false, fooExtraID: true})
}

func TestJournalContextV2AliasProjectionAndCompleteMutableFingerprint(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	tasks := make([]TaskCreateResult, 0, 2)
	for i := 0; i < 2; i++ {
		task, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: fmt.Sprintf("Task %d", i)})
		if err != nil {
			t.Fatalf("CreateTask(%d) error = %v", i, err)
		}
		tasks = append(tasks, task)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	deferrals := make([]JournalDeferResult, 0, 2)
	for i := 0; i < 2; i++ {
		item, err := store.DeferJournal(ctx, root, JournalDeferOptions{
			Intent: fmt.Sprintf("fingerprint %d", i), Why: "verify mutable cursor", Boundary: "wait", Trigger: "later", OperationID: fmt.Sprintf("fingerprint-%d", i),
			Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismSkill, Branch: "feature/aliases"},
		})
		if err != nil {
			t.Fatalf("DeferJournal(%d) error = %v", i, err)
		}
		deferrals = append(deferrals, item)
	}
	now := "2026-07-01T00:00:00Z"
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(aliases) error = %v", err)
	}
	if err := insertAlias(ctx, tx, projectID, "task", tasks[0].Task.ID, "task", "ALT-TASK", now); err != nil {
		tx.Rollback()
		t.Fatalf("insert task alias: %v", err)
	}
	if err := insertAlias(ctx, tx, projectID, "spark", deferrals[0].Spark.ID, "spark", "ALT-SPARK", now); err != nil {
		tx.Rollback()
		t.Fatalf("insert spark alias: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit aliases: %v", err)
	}

	options := JournalContextOptions{Branch: "feature/aliases", DeferralLimit: 1, TaskLimit: 1}
	first, err := store.JournalContext(ctx, root, options)
	if err != nil {
		t.Fatalf("JournalContext(aliases) error = %v", err)
	}
	if first.DeferredIntent.AvailableCount != 2 || first.TransitionalTasks.AvailableCount != 2 {
		t.Fatalf("alias projection duplicated candidates: deferrals=%d tasks=%d", first.DeferredIntent.AvailableCount, first.TransitionalTasks.AvailableCount)
	}
	assertJournalContextTaskPageUnion(t, ctx, store, root, options, first)

	assertStaleDeferralCursor := func(label string, before JournalContext, mutate func()) {
		t.Helper()
		mutate()
		_, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: options.Branch, DeferralLimit: 1, Cursor: before.DeferredIntent.Cursor, CursorLayer: JournalContextLayerDeferrals})
		var stale *JournalContextCursorStaleError
		if !errors.As(err, &stale) {
			t.Fatalf("%s mutation cursor error = %T %v, want stale", label, err, err)
		}
	}
	assertStaleDeferralCursor("alias", first, func() {
		if _, err := store.db.ExecContext(ctx, `UPDATE aliases SET alias = 'AAA-SPARK' WHERE project_id = ? AND entity_kind = 'spark' AND entity_id = ? AND namespace = 'spark' AND alias = 'ALT-SPARK'`, projectID, deferrals[0].Spark.ID); err != nil {
			t.Fatalf("mutate spark alias: %v", err)
		}
	})
	beforeText, err := store.JournalContext(ctx, root, options)
	if err != nil {
		t.Fatalf("JournalContext(before text mutation) error = %v", err)
	}
	assertStaleDeferralCursor("text", beforeText, func() {
		if _, err := store.db.ExecContext(ctx, `UPDATE sparks SET text = text || ' changed' WHERE project_id = ? AND id = ?`, projectID, deferrals[0].Spark.ID); err != nil {
			t.Fatalf("mutate spark text: %v", err)
		}
	})
	beforeOrigin, err := store.JournalContext(ctx, root, options)
	if err != nil {
		t.Fatalf("JournalContext(before origin mutation) error = %v", err)
	}
	assertStaleDeferralCursor("origin", beforeOrigin, func() {
		if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET branch = 'feature/changed' WHERE project_id = ? AND journal_entry_id = ?`, projectID, deferrals[0].Decision.ID); err != nil {
			t.Fatalf("mutate origin: %v", err)
		}
	})
}

func TestJournalContextV2SpecAliasProjectionDoesNotDuplicateTransitionalTask(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	spec, err := CreateSpec(ctx, root, resolver, SpecCreateOptions{Slug: "context-spec-alias"})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	linked, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: "Linked task", Spec: spec.Spec.Alias})
	if err != nil {
		t.Fatalf("CreateTask(linked) error = %v", err)
	}
	if _, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: "Unlinked task"}); err != nil {
		t.Fatalf("CreateTask(unlinked) error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(spec alias) error = %v", err)
	}
	if err := insertAlias(ctx, tx, projectID, "spec", spec.Spec.ID, "spec", "ALT-SPEC", "2026-07-01T00:00:00Z"); err != nil {
		tx.Rollback()
		t.Fatalf("insert alternate spec alias: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit alternate spec alias: %v", err)
	}

	options := JournalContextOptions{Branch: "feature/spec-alias", TaskLimit: 1}
	first, err := store.JournalContext(ctx, root, options)
	if err != nil {
		t.Fatalf("JournalContext(first task page) error = %v", err)
	}
	if first.TransitionalTasks.AvailableCount != 2 {
		t.Fatalf("AvailableCount = %d, want two tasks despite two aliases for one spec", first.TransitionalTasks.AvailableCount)
	}
	assertJournalContextTaskPageUnion(t, ctx, store, root, options, first)
	full, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: options.Branch, TaskLimit: 10})
	if err != nil {
		t.Fatalf("JournalContext(full task page) error = %v", err)
	}
	linkedCount := 0
	for _, task := range full.TransitionalTasks.Items {
		if task.Ref == linked.Task.Alias {
			linkedCount++
			if task.Spec != "ALT-SPEC" {
				t.Fatalf("linked task spec alias = %q, want deterministic MIN alias ALT-SPEC", task.Spec)
			}
		}
	}
	if linkedCount != 1 {
		t.Fatalf("linked task occurrences = %d, want exactly one", linkedCount)
	}
}

func assertJournalContextTaskPageUnion(t *testing.T, ctx context.Context, store *Store, root project.Root, options JournalContextOptions, first JournalContext) {
	t.Helper()
	seen := map[string]struct{}{}
	page := first
	for {
		for _, item := range page.TransitionalTasks.Items {
			if _, duplicate := seen[item.Ref]; duplicate {
				t.Fatalf("duplicate task page item %q", item.Ref)
			}
			seen[item.Ref] = struct{}{}
		}
		if page.TransitionalTasks.Cursor == "" {
			break
		}
		var err error
		page, err = store.JournalContext(ctx, root, JournalContextOptions{Branch: options.Branch, TaskLimit: options.TaskLimit, Cursor: page.TransitionalTasks.Cursor, CursorLayer: JournalContextLayerTasks})
		if err != nil {
			t.Fatalf("JournalContext(next task page) error = %v", err)
		}
	}
	if len(seen) != first.TransitionalTasks.AvailableCount {
		t.Fatalf("task page union = %d, want %d", len(seen), first.TransitionalTasks.AvailableCount)
	}
}

func assertJournalContextSourceAvailabilityJSON(t *testing.T, layer any, want bool) {
	t.Helper()
	data, err := json.Marshal(layer)
	if err != nil {
		t.Fatalf("json.Marshal(layer) error = %v", err)
	}
	wantField := fmt.Sprintf(`"source_available":%t`, want)
	if !strings.Contains(string(data), wantField) {
		t.Fatalf("layer JSON = %s, want %s", data, wantField)
	}
	if strings.Contains(string(data), `"available":`) {
		t.Fatalf("layer JSON = %s, legacy available field present", data)
	}
}

// seedRawLegacyDeferral inserts a pre-conversion journal_deferrals row (a
// decision, an open spark, and the deferral pair) with no intent_operations
// mapping and caller-controlled branch/text/timestamp, mirroring state written
// before the canonical Intent model existed.
func seedRawLegacyDeferral(t *testing.T, store *Store, projectID, operationKey, intentText, sparkText, branch, createdAt string) (string, string) {
	t.Helper()
	decisionID := stableMigrationID("test-legacy-decision", projectID, operationKey)
	sparkID := stableMigrationID("test-legacy-spark", projectID, operationKey)
	scope := "defer/" + operationKey
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  session_id, spec_id, task_id, created_at, updated_at
) VALUES (?, ?, 'decision', ?, ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?)
`, decisionID, projectID, scope, "Intent: "+intentText, emptyToNil(branch), createdAt, createdAt); err != nil {
		t.Fatalf("seed legacy decision: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, 'open', ?, NULL, ?, ?)
`, sparkID, projectID, scope, sparkText, createdAt, createdAt); err != nil {
		t.Fatalf("seed legacy spark: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, operationKey, decisionID, sparkID, strings.Repeat("b", 64), createdAt); err != nil {
		t.Fatalf("seed legacy deferral: %v", err)
	}
	return decisionID, sparkID
}

// seedExplorationCheckpoint inserts one immutable checkpoint with a controlled
// sequence and timestamp so tests can assert greatest-sequence latest selection
// and deterministic recency ordering independently of wall-clock append time.
func seedExplorationCheckpoint(t *testing.T, store *Store, projectID, explorationID string, seq int, nextAction, createdAt string) {
	t.Helper()
	id := stableMigrationID("test-checkpoint", projectID, explorationID, fmt.Sprintf("%d", seq))
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at)
VALUES (?, ?, ?, ?, 'purpose', 'conclusions', 'unresolved', ?, ?, ?)
`, id, projectID, explorationID, seq, nextAction, strings.Repeat("a", 64), createdAt); err != nil {
		t.Fatalf("seed exploration checkpoint: %v", err)
	}
}

func TestJournalContextCanonicalDeferredIntentIsActiveTruthAheadOfLegacy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	canonical, err := store.CreateIntent(ctx, root, IntentCreateOptions{
		Title: "Rework retry budget", Body: "revisit the retry budget model",
		Disposition: "deferred", Why: "needs data first", Boundary: "do not touch prod", Trigger: "when metrics land",
		OperationID: "canonical-active-truth",
	})
	if err != nil {
		t.Fatalf("CreateIntent(deferred) error = %v", err)
	}

	projectID := projectIDForTest(t, store, root)
	// A pre-conversion legacy deferral whose decision is much newer than the
	// canonical Intent, plus a flood of newer journal noise.
	legacyDecision, legacySpark := seedRawLegacyDeferral(t, store, projectID, "legacy-active-truth", "legacy direction", "legacy spark text", "", "2026-07-09T12:00:00Z")
	for i := 0; i < 40; i++ {
		seedJournalEntry(t, store, projectID, "discover", "noise", fmt.Sprintf("noise-%02d", i), "main", fmt.Sprintf("2026-07-10T%02d:00:00Z", i%24))
	}

	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.DeferredIntent.AvailableCount != 2 || len(result.DeferredIntent.Items) != 2 {
		t.Fatalf("deferred intent count = %d, want 2 (canonical + legacy)", result.DeferredIntent.AvailableCount)
	}
	first := result.DeferredIntent.Items[0]
	if first.Intent == nil || first.Intent.Alias != canonical.Intent.Alias {
		t.Fatalf("first deferred item = %#v, want canonical intent %s ahead of newer legacy", first, canonical.Intent.Alias)
	}
	if first.Intent.Body != "revisit the retry budget model" || first.Intent.Why != "needs data first" || first.Intent.Boundary != "do not touch prod" || first.Intent.RevisitTrigger != "when metrics land" {
		t.Fatalf("canonical intent packet = %#v, want the self-sufficient deferral fields", first.Intent)
	}
	if first.Intent.Disposition != "deferred" {
		t.Fatalf("canonical disposition = %q, want deferred", first.Intent.Disposition)
	}
	// A version-0 canonical Intent has no legacy projection: never fabricate one.
	if first.Decision.ID != "" || first.Spark.ID != "" {
		t.Fatalf("canonical item fabricated a legacy projection: decision=%q spark=%q", first.Decision.ID, first.Spark.ID)
	}
	second := result.DeferredIntent.Items[1]
	if second.Intent != nil || second.OperationKey != "legacy-active-truth" {
		t.Fatalf("second deferred item = %#v, want the legacy-only deferral", second)
	}
	if second.Decision.ID != legacyDecision || second.Spark.ID != legacySpark {
		t.Fatalf("legacy item projection = decision %q spark %q, want %q/%q", second.Decision.ID, second.Spark.ID, legacyDecision, legacySpark)
	}
}

func TestJournalContextDeferJournalAdapterAppearsOnceWithEnrichment(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	deferred, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "adapter deferral", Why: "later", Boundary: "not now", Trigger: "on resume", OperationID: "adapter-once",
		Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismSkill, Branch: "feat/x"},
	})
	if err != nil {
		t.Fatalf("DeferJournal() error = %v", err)
	}
	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "feat/x"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.DeferredIntent.AvailableCount != 1 || len(result.DeferredIntent.Items) != 1 {
		t.Fatalf("adapter deferral surfaced %d items, want exactly one canonical item", result.DeferredIntent.AvailableCount)
	}
	item := result.DeferredIntent.Items[0]
	if item.Intent == nil || item.Intent.Alias != deferred.IntentAlias {
		t.Fatalf("deduplicated item = %#v, want canonical intent %s", item, deferred.IntentAlias)
	}
	if item.Decision.ID != deferred.Decision.ID || item.Spark.ID != deferred.Spark.ID {
		t.Fatalf("version-1 enrichment = decision %q spark %q, want %q/%q", item.Decision.ID, item.Spark.ID, deferred.Decision.ID, deferred.Spark.ID)
	}
	if item.Origin == nil || item.Origin.Branch != "feat/x" {
		t.Fatalf("origin enrichment = %#v, want the mapped decision origin", item.Origin)
	}
}

func TestJournalContextPreConversionLegacyDeferralAppears(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	decisionID, sparkID := seedRawLegacyDeferral(t, store, projectID, "pre-conversion", "unmigrated direction", "unmigrated spark", "", "2026-07-01T09:00:00Z")
	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.DeferredIntent.AvailableCount != 1 || len(result.DeferredIntent.Items) != 1 {
		t.Fatalf("legacy deferral count = %d, want 1", result.DeferredIntent.AvailableCount)
	}
	item := result.DeferredIntent.Items[0]
	if item.Intent != nil {
		t.Fatalf("legacy item carried an intent sub-object: %#v", item.Intent)
	}
	if item.OperationKey != "pre-conversion" || item.Decision.ID != decisionID || item.Spark.ID != sparkID {
		t.Fatalf("legacy item = %#v, want operation pre-conversion with mapped decision/spark", item)
	}
}

func TestJournalContextResolvedAndResumedIntentsDisappear(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	deferrals := []struct{ op, title string }{{"defer-resolve", "resolve me"}, {"defer-resume", "resume me"}, {"defer-keep", "keep me"}}
	created := map[string]IntentMutationResult{}
	for _, spec := range deferrals {
		res, err := store.CreateIntent(ctx, root, IntentCreateOptions{Title: spec.title, Body: spec.title + " body", Disposition: "deferred", Why: "w", Boundary: "b", Trigger: "t", OperationID: spec.op})
		if err != nil {
			t.Fatalf("CreateIntent(%s) error = %v", spec.op, err)
		}
		created[spec.op] = res
	}
	if _, err := store.appendIntentDisposition(ctx, root, IntentDispositionOptions{IntentRef: created["defer-resolve"].Intent.Alias, Reason: "done exploring"}, "resolved"); err != nil {
		t.Fatalf("resolve error = %v", err)
	}
	if _, err := store.appendIntentDisposition(ctx, root, IntentDispositionOptions{IntentRef: created["defer-resume"].Intent.Alias, Reason: "back on it"}, "tracked"); err != nil {
		t.Fatalf("resume error = %v", err)
	}
	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	if result.DeferredIntent.AvailableCount != 1 || len(result.DeferredIntent.Items) != 1 {
		t.Fatalf("deferred count = %d, want only the still-deferred intent", result.DeferredIntent.AvailableCount)
	}
	if got := result.DeferredIntent.Items[0].Intent; got == nil || got.Alias != created["defer-keep"].Intent.Alias {
		t.Fatalf("surviving deferred item = %#v, want %s", got, created["defer-keep"].Intent.Alias)
	}
}

func TestJournalContextExplorationCheckpointsLayerBoundsLatestPerExploration(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)

	older, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Older inquiry"})
	if err != nil {
		t.Fatalf("CreateExploration(older) error = %v", err)
	}
	newer, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Newer inquiry"})
	if err != nil {
		t.Fatalf("CreateExploration(newer) error = %v", err)
	}
	// A handle-only Exploration with no checkpoint must never surface: a handle
	// is never proof of portable context.
	if _, err := store.CreateExploration(ctx, root, ExplorationCreateOptions{Title: "Handle only"}); err != nil {
		t.Fatalf("CreateExploration(handle only) error = %v", err)
	}

	// The greater sequence is the latest even when its timestamp is older.
	seedExplorationCheckpoint(t, store, projectID, older.Exploration.ID, 1, "older superseded action", "2026-07-01T08:00:00Z")
	seedExplorationCheckpoint(t, store, projectID, older.Exploration.ID, 2, "older latest action", "2026-07-01T07:00:00Z")
	seedExplorationCheckpoint(t, store, projectID, newer.Exploration.ID, 1, "newer latest action", "2026-07-02T09:00:00Z")

	result, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("JournalContext() error = %v", err)
	}
	layer := result.ExplorationCheckpoints
	if !layer.Available || layer.AvailableCount != 2 || layer.ShownCount != 2 || layer.Truncated {
		t.Fatalf("checkpoints layer = %#v, want two checkpointed explorations, none truncated", layer)
	}
	if layer.Items[0].ExplorationID != newer.Exploration.ID || layer.Items[1].ExplorationID != older.Exploration.ID {
		t.Fatalf("checkpoint order = %#v, want newest checkpoint first", layer.Items)
	}
	if layer.Items[1].Seq != 2 || layer.Items[1].NextAction != "older latest action" {
		t.Fatalf("older latest checkpoint = %#v, want the greatest-sequence row", layer.Items[1])
	}
	if want := "loaf exploration context " + newer.Exploration.Alias; layer.Items[0].ContextCommand != want {
		t.Fatalf("context command = %q, want %q", layer.Items[0].ContextCommand, want)
	}
	assertJournalContextSourceAvailabilityJSON(t, layer, true)

	bounded, err := store.JournalContext(ctx, root, JournalContextOptions{Branch: "main", CheckpointsLimit: 1})
	if err != nil {
		t.Fatalf("JournalContext(bounded) error = %v", err)
	}
	checkpoints := bounded.ExplorationCheckpoints
	if checkpoints.AvailableCount != 2 || checkpoints.ShownCount != 1 || !checkpoints.Truncated {
		t.Fatalf("bounded checkpoints = %#v, want 1 of 2 truncated", checkpoints)
	}
	if want := "loaf journal context --layer exploration-checkpoints --limit 1 --branch 'main'"; checkpoints.ExpandCommand != want {
		t.Fatalf("expand command = %q, want %q", checkpoints.ExpandCommand, want)
	}
	if checkpoints.Cursor != "" {
		t.Fatalf("checkpoints layer must not expose a cursor, got %q", checkpoints.Cursor)
	}
}

func assertJournalContextEntryIDs(t *testing.T, entries []JournalEntryRecord, expected map[string]bool) {
	t.Helper()
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.ID] = true
	}
	for id, want := range expected {
		if seen[id] != want {
			t.Fatalf("entry %s presence = %t, want %t; entries=%#v", id, seen[id], want, entries)
		}
	}
}
