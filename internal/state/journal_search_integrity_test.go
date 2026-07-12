package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestInspectJournalSearchParityReadyAndNonMutating(t *testing.T) {
	ctx := context.Background()
	store, _ := openJournalParityStore(t)
	defer store.Close()

	before, err := journalSearchStateSnapshot(ctx, store)
	if err != nil {
		t.Fatalf("journalSearchStateSnapshot(before) error = %v", err)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if want := (JournalSearchParity{CanonicalRows: 1, IndexRows: 1, Ready: true}); parity != want {
		t.Fatalf("parity = %#v, want %#v", parity, want)
	}
	after, err := journalSearchStateSnapshot(ctx, store)
	if err != nil {
		t.Fatalf("journalSearchStateSnapshot(after) error = %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("parity inspection mutated derived rows: before=%#v after=%#v", before, after)
	}
}

func TestInspectJournalSearchParityClassifiesDivergence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, context.Context, *Store, string, string)
		want   JournalSearchParity
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, ctx context.Context, store *Store, _, journalID string) {
				if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
					t.Fatalf("delete derived row: %v", err)
				}
			},
			want: JournalSearchParity{CanonicalRows: 1, IndexRows: 0, Missing: 1},
		},
		{
			name: "extra",
			mutate: func(t *testing.T, ctx context.Context, store *Store, projectID, _ string) {
				correlationColumn, err := journalSearchCorrelationColumn(ctx, store)
				if err != nil {
					t.Fatalf("journalSearchCorrelationColumn: %v", err)
				}
				query := fmt.Sprintf(`INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
VALUES ((SELECT COALESCE(MAX(rowid), 0) + 1 FROM journal_search), ?, 'rogue-journal', '', 'rogue', '', 'rogue')`, correlationColumn)
				if _, err := store.db.ExecContext(ctx, query, projectID); err != nil {
					t.Fatalf("insert rogue row: %v", err)
				}
			},
			want: JournalSearchParity{CanonicalRows: 1, IndexRows: 2, Extra: 1},
		},
		{
			name: "changed",
			mutate: func(t *testing.T, ctx context.Context, store *Store, _, journalID string) {
				if _, err := store.db.ExecContext(ctx, `UPDATE journal_search SET message = 'changed' WHERE journal_entry_id = ?`, journalID); err != nil {
					t.Fatalf("update derived row: %v", err)
				}
			},
			want: JournalSearchParity{CanonicalRows: 1, IndexRows: 1, Changed: 1},
		},
		{
			name: "rowid-mismatch",
			mutate: func(t *testing.T, ctx context.Context, store *Store, _, journalID string) {
				correlationColumn, err := journalSearchCorrelationColumn(ctx, store)
				if err != nil {
					t.Fatalf("journalSearchCorrelationColumn: %v", err)
				}
				if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
					t.Fatalf("delete derived row: %v", err)
				}
				query := fmt.Sprintf(`INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
SELECT (SELECT COALESCE(MAX(rowid), 0) + 100 FROM journal_search), project_id, id, COALESCE(%s, ''), entry_type, COALESCE(scope, ''), message
FROM journal_entries WHERE id = ?`, correlationColumn, correlationColumn)
				if _, err := store.db.ExecContext(ctx, query, journalID); err != nil {
					t.Fatalf("insert rowid-mismatched row: %v", err)
				}
			},
			want: JournalSearchParity{CanonicalRows: 1, IndexRows: 1, Changed: 1},
		},
		{
			name: "duplicate-derived-row",
			mutate: func(t *testing.T, ctx context.Context, store *Store, projectID, journalID string) {
				correlationColumn, err := journalSearchCorrelationColumn(ctx, store)
				if err != nil {
					t.Fatalf("journalSearchCorrelationColumn: %v", err)
				}
				query := fmt.Sprintf(`INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
SELECT (SELECT COALESCE(MAX(rowid), 0) + 1 FROM journal_search), project_id, journal_entry_id, %s, entry_type, scope, message
FROM journal_search WHERE project_id = ? AND journal_entry_id = ?`, correlationColumn, correlationColumn)
				if _, err := store.db.ExecContext(ctx, query, projectID, journalID); err != nil {
					t.Fatalf("duplicate derived row: %v", err)
				}
			},
			want: JournalSearchParity{CanonicalRows: 1, IndexRows: 2, Extra: 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, root := openJournalParityStore(t)
			defer store.Close()
			projectID := projectIDForTest(t, store, root)
			var journalID string
			if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
				t.Fatalf("read journal ID: %v", err)
			}
			test.mutate(t, ctx, store, projectID, journalID)
			parity, err := InspectJournalSearchParity(ctx, store)
			if err != nil {
				t.Fatalf("InspectJournalSearchParity() error = %v", err)
			}
			if parity != test.want {
				t.Fatalf("parity = %#v, want %#v", parity, test.want)
			}
		})
	}
}

func TestJournalSearchDivergenceRefusesSearchWithTypedRepairError(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(context.Context, *Store, string) error
	}{
		{
			name: "missing",
			mutate: func(ctx context.Context, store *Store, journalID string) error {
				_, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID)
				return err
			},
		},
		{
			name: "extra",
			mutate: func(ctx context.Context, store *Store, _ string) error {
				correlationColumn, err := journalSearchCorrelationColumn(ctx, store)
				if err != nil {
					return err
				}
				query := fmt.Sprintf(`INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
VALUES ((SELECT COALESCE(MAX(rowid), 0) + 1 FROM journal_search), 'rogue-project', 'rogue-journal', '', 'rogue', '', 'rogue')`, correlationColumn)
				_, err = store.db.ExecContext(ctx, query)
				return err
			},
		},
		{
			name: "changed",
			mutate: func(ctx context.Context, store *Store, journalID string) error {
				_, err := store.db.ExecContext(ctx, `UPDATE journal_search SET message = 'changed' WHERE journal_entry_id = ?`, journalID)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store, root := openJournalParityStore(t)
			defer store.Close()
			var journalID string
			if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
				t.Fatalf("read journal ID: %v", err)
			}
			if err := test.mutate(ctx, store, journalID); err != nil {
				t.Fatalf("mutate %s: %v", test.name, err)
			}
			_, err := store.SearchJournal(ctx, root, SearchOptions{Query: "canonical"})
			if err == nil {
				t.Fatal("SearchJournal() error = nil, want divergence refusal")
			}
			var divergence *JournalSearchDivergenceError
			if !errors.As(err, &divergence) {
				t.Fatalf("SearchJournal() error = %v, want JournalSearchDivergenceError", err)
			}
			if divergence.Code != JournalSearchDivergenceCode {
				t.Fatalf("divergence code = %q, want %q", divergence.Code, JournalSearchDivergenceCode)
			}
			if !strings.Contains(err.Error(), "loaf state repair journal-search --dry-run") {
				t.Fatalf("divergence error = %v, want exact repair command", err)
			}
		})
	}
}

func TestGlobalSearchDivergenceGuardsBeforeDocsWork(t *testing.T) {
	ctx := context.Background()
	store, root := openJournalParityStore(t)
	defer store.Close()
	var journalID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
		t.Fatalf("read journal ID: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
		t.Fatalf("delete derived row: %v", err)
	}
	writeBrokenDocsFile(t, root.Path(), "docs/broken.md")
	beforeBytes := durableSQLiteFilesSnapshot(t, store.Path())
	var docsRowsBefore int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs_index`).Scan(&docsRowsBefore); err != nil {
		t.Fatalf("count docs rows before search: %v", err)
	}

	_, err := store.Search(ctx, root, SearchOptions{Query: "canonical"})
	if err == nil {
		t.Fatal("Search() error = nil, want divergence refusal before docs work")
	}
	var divergence *JournalSearchDivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("Search() error = %v, want JournalSearchDivergenceError", err)
	}
	afterBytes := durableSQLiteFilesSnapshot(t, store.Path())
	if !reflect.DeepEqual(beforeBytes, afterBytes) {
		t.Fatalf("global search mutated database bytes before refusal")
	}
	var docsRowsAfter int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs_index`).Scan(&docsRowsAfter); err != nil {
		t.Fatalf("count docs rows after search: %v", err)
	}
	if docsRowsAfter != docsRowsBefore {
		t.Fatalf("docs_index rows changed on guarded search: before=%d after=%d", docsRowsBefore, docsRowsAfter)
	}
}

func TestJournalParityDivergenceInOtherProjectDoesNotBlockCanonicalJournalOperations(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	currentRoot := projectRoot(t)
	otherRoot := projectRoot(t)
	if _, err := Initialize(ctx, currentRoot, resolver); err != nil {
		t.Fatalf("Initialize(current) error = %v", err)
	}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	if _, err := LogJournal(ctx, otherRoot, resolver, JournalLogOptions{Entry: "discover(other): other project entry"}); err != nil {
		t.Fatalf("LogJournal(other) error = %v", err)
	}

	databasePath, err := resolver.DatabasePath(currentRoot)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	otherProjectID := projectIDForTest(t, store, otherRoot)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE project_id = ?`, otherProjectID); err != nil {
		store.Close()
		t.Fatalf("delete other project journal search row: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() after corruption error = %v", err)
	}

	if _, err := JournalContextForRoot(ctx, currentRoot, resolver, JournalContextOptions{}); err != nil {
		t.Fatalf("JournalContextForRoot(current) error = %v", err)
	}
	recent, err := RecentJournal(ctx, currentRoot, resolver, JournalRecentOptions{})
	if err != nil {
		t.Fatalf("RecentJournal(current) error = %v", err)
	}
	if len(recent.Entries) != 0 {
		t.Fatalf("RecentJournal(current) entries = %#v, want empty before current write", recent.Entries)
	}
	if _, err := LogJournal(ctx, currentRoot, resolver, JournalLogOptions{Entry: "decision(current): current project entry"}); err != nil {
		t.Fatalf("LogJournal(current) error = %v", err)
	}

	parityStore, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly() error = %v", err)
	}
	parity, err := InspectJournalSearchParity(ctx, parityStore)
	if closeErr := parityStore.Close(); closeErr != nil {
		t.Fatalf("Close() parity store error = %v", closeErr)
	}
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if parity.Ready || parity.CanonicalRows != 2 || parity.IndexRows != 1 || parity.Missing != 1 {
		t.Fatalf("global parity = %#v, want canonical=2/index=1/missing=1 and not ready", parity)
	}

	_, err = SearchJournal(ctx, currentRoot, resolver, SearchOptions{Query: "current"})
	if err == nil {
		t.Fatal("SearchJournal(current) error = nil, want typed divergence refusal")
	}
	var divergence *JournalSearchDivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("SearchJournal(current) error = %v, want JournalSearchDivergenceError", err)
	}
	if divergence.Code != JournalSearchDivergenceCode || divergence.Parity != parity {
		t.Fatalf("SearchJournal(current) divergence = %#v, want code=%q parity=%#v", divergence, JournalSearchDivergenceCode, parity)
	}
}

func TestTopLevelJournalSearchReturnsStructuredDivergence(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(parity): canonical message"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		store.Close()
		t.Fatalf("delete journal_search: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err := SearchJournal(ctx, root, resolver, SearchOptions{Query: "canonical"})
	var divergence *JournalSearchDivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("SearchJournal() error = %v, want JournalSearchDivergenceError", err)
	}
	if divergence.Code != JournalSearchDivergenceCode || divergence.Parity.CanonicalRows != 1 || divergence.Parity.IndexRows != 0 || divergence.Parity.Missing != 1 {
		t.Fatalf("divergence = %#v, want code and exact missing-index counts", divergence)
	}
}

func TestConcurrentJournalWritesAndParityInspectionRemainReady(t *testing.T) {
	const (
		writers          = 6
		entriesPerWriter = 12
	)
	total := writers * entriesPerWriter
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	start := make(chan struct{})
	firstCommitted := make(chan struct{})
	var firstCommitOnce sync.Once
	var committed atomic.Int32
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			<-start
			for entry := 0; entry < entriesPerWriter; entry++ {
				_, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: fmt.Sprintf("discover(stress): writer-%02d-entry-%02d", writer, entry)})
				if err != nil {
					errCh <- err
					return
				}
				if committed.Add(1) == 1 {
					firstCommitOnce.Do(func() { close(firstCommitted) })
				}
				runtime.Gosched()
				time.Sleep(250 * time.Microsecond)
			}
		}(writer)
	}
	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-firstCommitted:
	case <-done:
		close(errCh)
		for err := range errCh {
			t.Fatalf("concurrent LogJournal() error = %v", err)
		}
		t.Fatal("concurrent writers finished without a committed entry")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the first committed journal entry")
	}

	seenIntermediate := make(map[int]struct{})
inspectionLoop:
	for {
		count := int(committed.Load())
		if count > 0 && count < total {
			parity, err := InspectJournalSearchParity(ctx, store)
			if err != nil {
				t.Fatalf("InspectJournalSearchParity() error = %v", err)
			}
			if !parity.Ready {
				t.Fatalf("observed non-ready parity at committed=%d: %#v", count, parity)
			}
			seenIntermediate[count] = struct{}{}
		}
		select {
		case <-done:
			break inspectionLoop
		default:
			runtime.Gosched()
			time.Sleep(250 * time.Microsecond)
		}
	}
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent LogJournal() error = %v", err)
	}
	if got := int(committed.Load()); got != total {
		t.Fatalf("committed entries = %d, want %d", got, total)
	}
	if len(seenIntermediate) < 2 {
		t.Fatalf("observed %d distinct intermediate committed counts, want at least 2", len(seenIntermediate))
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("final InspectJournalSearchParity() error = %v", err)
	}
	if parity != (JournalSearchParity{CanonicalRows: total, IndexRows: total, Ready: true}) {
		t.Fatalf("final parity = %#v, want %d rows and ready", parity, total)
	}
}

func TestInspectJournalSearchParitySupportsPreAndPostJournalFirstShapes(t *testing.T) {
	ctx := context.Background()
	preRoot := projectRoot(t)
	preHome := t.TempDir()
	preResolver := PathResolver{StateHome: preHome}
	preStatus, err := Initialize(ctx, preRoot, preResolver)
	if err != nil {
		t.Fatalf("pre Initialize() error = %v", err)
	}
	preStore := openTestStore(t, preRoot, preHome)
	now := "2026-07-10T00:00:00Z"
	if _, err := preStore.db.ExecContext(ctx, `
INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at)
VALUES ('legacy-session', ?, NULL, 'main', 'active', NULL, ?, ?)`, preStatus.ProjectID, now, now); err != nil {
		preStore.Close()
		t.Fatalf("pre legacy session insert error = %v", err)
	}
	preHarness := "pre-harness"
	if _, err := preStore.db.ExecContext(ctx, journalInsertSQL, journalArgs("pre-harness-entry", preStatus.ProjectID, "decision", "pre", "pre harness", "main", "", &preHarness, now)...); err != nil {
		preStore.Close()
		t.Fatalf("pre harness journal insert error = %v", err)
	}
	emptyHarness := ""
	if _, err := preStore.db.ExecContext(ctx, journalInsertSQL, journalArgs("pre-legacy-entry", preStatus.ProjectID, "note", "pre", "legacy session", "main", "legacy-session", &emptyHarness, now)...); err != nil {
		preStore.Close()
		t.Fatalf("pre legacy journal insert error = %v", err)
	}
	tx, err := preStore.db.BeginTx(ctx, nil)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre rebuild BeginTx() error = %v", err)
	}
	if _, err := rebuildJournalSearchTx(ctx, tx); err != nil {
		tx.Rollback()
		preStore.Close()
		t.Fatalf("pre rebuildJournalSearchTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		preStore.Close()
		t.Fatalf("pre rebuild Commit() error = %v", err)
	}
	preParity, err := InspectJournalSearchParity(ctx, preStore)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre InspectJournalSearchParity() error = %v", err)
	}
	if preParity != (JournalSearchParity{CanonicalRows: 2, IndexRows: 2, Ready: true}) {
		preStore.Close()
		t.Fatalf("pre parity = %#v, want two rows and ready", preParity)
	}
	preCorrelation, err := journalSearchCorrelationColumn(ctx, preStore)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre correlation column error = %v", err)
	}
	if preCorrelation != "session_id" {
		preStore.Close()
		t.Fatalf("pre correlation column = %q, want session_id", preCorrelation)
	}
	var preDerivedHarness, preDerivedLegacy string
	if err := preStore.db.QueryRowContext(ctx, `SELECT session_id FROM journal_search WHERE journal_entry_id = 'pre-harness-entry'`).Scan(&preDerivedHarness); err != nil {
		preStore.Close()
		t.Fatalf("pre derived harness read error = %v", err)
	}
	if err := preStore.db.QueryRowContext(ctx, `SELECT session_id FROM journal_search WHERE journal_entry_id = 'pre-legacy-entry'`).Scan(&preDerivedLegacy); err != nil {
		preStore.Close()
		t.Fatalf("pre derived legacy read error = %v", err)
	}
	if preDerivedHarness != "pre-harness" {
		preStore.Close()
		t.Fatalf("pre derived harness session_id = %q, want pre-harness", preDerivedHarness)
	}
	if preDerivedLegacy != "legacy-session" {
		preStore.Close()
		t.Fatalf("pre derived legacy session_id = %q, want legacy-session", preDerivedLegacy)
	}
	if _, err := preStore.db.ExecContext(ctx, `UPDATE journal_search SET session_id = 'corrupted' WHERE journal_entry_id = 'pre-harness-entry'`); err != nil {
		preStore.Close()
		t.Fatalf("pre derived corruption error = %v", err)
	}
	preParity, err = InspectJournalSearchParity(ctx, preStore)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre InspectJournalSearchParity() after corruption error = %v", err)
	}
	if preParity.Changed != 1 || preParity.Ready {
		preStore.Close()
		t.Fatalf("pre parity after corruption = %#v, want changed=1 and not ready", preParity)
	}
	tx, err = preStore.db.BeginTx(ctx, nil)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre rebuild after corruption BeginTx() error = %v", err)
	}
	if _, err := rebuildJournalSearchTx(ctx, tx); err != nil {
		tx.Rollback()
		preStore.Close()
		t.Fatalf("pre rebuild after corruption error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		preStore.Close()
		t.Fatalf("pre rebuild after corruption Commit() error = %v", err)
	}
	preParity, err = InspectJournalSearchParity(ctx, preStore)
	if err != nil {
		preStore.Close()
		t.Fatalf("pre InspectJournalSearchParity() after rebuild error = %v", err)
	}
	if preParity != (JournalSearchParity{CanonicalRows: 2, IndexRows: 2, Ready: true}) {
		preStore.Close()
		t.Fatalf("pre parity after rebuild = %#v, want two rows and ready", preParity)
	}
	if err := preStore.db.QueryRowContext(ctx, `SELECT session_id FROM journal_search WHERE journal_entry_id = 'pre-harness-entry'`).Scan(&preDerivedHarness); err != nil {
		preStore.Close()
		t.Fatalf("pre restored harness read error = %v", err)
	}
	if preDerivedHarness != "pre-harness" {
		preStore.Close()
		t.Fatalf("pre restored derived harness session_id = %q, want pre-harness", preDerivedHarness)
	}
	preStore.Close()

	postRoot := projectRoot(t)
	postHome := t.TempDir()
	postResolver := PathResolver{StateHome: postHome}
	postStatus, err := Initialize(ctx, postRoot, postResolver)
	if err != nil {
		t.Fatalf("post Initialize() error = %v", err)
	}
	postStoreBefore := openTestStore(t, postRoot, postHome)
	postHarness := "post-harness"
	if _, err := postStoreBefore.db.ExecContext(ctx, journalInsertSQL, journalArgs("post-harness-entry", postStatus.ProjectID, "decision", "post", "post harness", "main", "", &postHarness, now)...); err != nil {
		postStoreBefore.Close()
		t.Fatalf("post harness journal insert error = %v", err)
	}
	postStoreBefore.Close()
	if _, err := ApplyJournalFirstMigration(ctx, postRoot, postResolver); err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}
	postStore := openTestStore(t, postRoot, postHome)
	defer postStore.Close()
	postParity, err := InspectJournalSearchParity(ctx, postStore)
	if err != nil {
		t.Fatalf("post InspectJournalSearchParity() error = %v", err)
	}
	if !postParity.Ready {
		t.Fatalf("post parity = %#v, want ready", postParity)
	}
	postCorrelation, err := journalSearchCorrelationColumn(ctx, postStore)
	if err != nil {
		t.Fatalf("post correlation column error = %v", err)
	}
	if postCorrelation != "harness_session_id" {
		t.Fatalf("post correlation column = %q, want harness_session_id", postCorrelation)
	}
	var postDerivedHarness string
	if err := postStore.db.QueryRowContext(ctx, `SELECT harness_session_id FROM journal_search WHERE journal_entry_id = 'post-harness-entry'`).Scan(&postDerivedHarness); err != nil {
		t.Fatalf("post derived harness read error = %v", err)
	}
	if postDerivedHarness != "post-harness" {
		t.Fatalf("post derived harness_session_id = %q, want post-harness", postDerivedHarness)
	}
	if _, err := postStore.db.ExecContext(ctx, `UPDATE journal_search SET harness_session_id = 'corrupted' WHERE journal_entry_id = 'post-harness-entry'`); err != nil {
		t.Fatalf("post derived corruption error = %v", err)
	}
	postParity, err = InspectJournalSearchParity(ctx, postStore)
	if err != nil {
		t.Fatalf("post InspectJournalSearchParity() after corruption error = %v", err)
	}
	if postParity.Changed != 1 || postParity.Ready {
		t.Fatalf("post parity after corruption = %#v, want changed=1 and not ready", postParity)
	}
	tx, err = postStore.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("post rebuild after corruption BeginTx() error = %v", err)
	}
	if _, err := rebuildJournalSearchTx(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatalf("post rebuild after corruption error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("post rebuild after corruption Commit() error = %v", err)
	}
	postParity, err = InspectJournalSearchParity(ctx, postStore)
	if err != nil {
		t.Fatalf("post InspectJournalSearchParity() after rebuild error = %v", err)
	}
	if postParity != (JournalSearchParity{CanonicalRows: 1, IndexRows: 1, Ready: true}) {
		t.Fatalf("post parity after rebuild = %#v, want one row and ready", postParity)
	}
	if err := postStore.db.QueryRowContext(ctx, `SELECT harness_session_id FROM journal_search WHERE journal_entry_id = 'post-harness-entry'`).Scan(&postDerivedHarness); err != nil {
		t.Fatalf("post restored harness read error = %v", err)
	}
	if postDerivedHarness != "post-harness" {
		t.Fatalf("post restored derived harness_session_id = %q, want post-harness", postDerivedHarness)
	}
}

func TestRebuildJournalSearchTxRestoresExactParityAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, _ := openJournalParityStore(t)
	defer store.Close()
	var journalID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM journal_entries LIMIT 1`).Scan(&journalID); err != nil {
		t.Fatalf("read journal ID: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, journalID); err != nil {
		t.Fatalf("delete derived row: %v", err)
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	rebuilt, err := rebuildJournalSearchTx(ctx, tx)
	if err != nil {
		tx.Rollback()
		t.Fatalf("rebuildJournalSearchTx() error = %v", err)
	}
	if rebuilt != 1 {
		tx.Rollback()
		t.Fatalf("rebuilt = %d, want 1", rebuilt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("rebuilt parity = %#v, want ready", parity)
	}
	before, err := journalSearchStateSnapshot(ctx, store)
	if err != nil {
		t.Fatalf("snapshot(before idempotent rebuild) error = %v", err)
	}
	tx, err = store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(idempotent) error = %v", err)
	}
	if rebuilt, err := rebuildJournalSearchTx(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatalf("idempotent rebuild error = %v", err)
	} else if rebuilt != 1 {
		tx.Rollback()
		t.Fatalf("idempotent rebuilt = %d, want 1", rebuilt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(idempotent) error = %v", err)
	}
	after, err := journalSearchStateSnapshot(ctx, store)
	if err != nil {
		t.Fatalf("snapshot(after idempotent rebuild) error = %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("idempotent rebuild changed rows: before=%#v after=%#v", before, after)
	}
}

func openJournalParityStore(t *testing.T) (*Store, project.Root) {
	t.Helper()
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "decision(parity): canonical message"}); err != nil {
		store.Close()
		t.Fatalf("LogJournal() error = %v", err)
	}
	return store, root
}

func journalSearchStateSnapshot(ctx context.Context, store *Store) ([]string, error) {
	correlationColumn, err := journalSearchCorrelationColumn(ctx, store)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`
SELECT rowid, project_id, journal_entry_id, COALESCE(%s, ''), entry_type, COALESCE(scope, ''), message
FROM journal_search
ORDER BY project_id, journal_entry_id, rowid`, correlationColumn)
	rows, err := store.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var rowid int64
		var projectID, journalEntryID, correlation, entryType, scope, message string
		if err := rows.Scan(&rowid, &projectID, &journalEntryID, &correlation, &entryType, &scope, &message); err != nil {
			return nil, err
		}
		snapshot = append(snapshot, fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", rowid, projectID, journalEntryID, correlation, entryType, scope, message))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// durableSQLiteFilesSnapshot captures only durable SQLite files: the main
// database and any pre-existing WAL. SQLite's -shm file is ephemeral
// coordination state and is intentionally excluded.
func durableSQLiteFilesSnapshot(t *testing.T, path string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte)
	for _, candidate := range []string{path, path + "-wal"} {
		contents, err := os.ReadFile(candidate)
		if err == nil {
			snapshot[candidate] = contents
			continue
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read database snapshot %s: %v", candidate, err)
		}
	}
	return snapshot
}

func BenchmarkInspectJournalSearchParity(b *testing.B) {
	const entries = 5000
	ctx := context.Background()
	root, err := project.ResolveRoot(b.TempDir())
	if err != nil {
		b.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := b.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		b.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStoreBenchmark(b, root, stateHome)
	defer store.Close()
	var projectID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM projects LIMIT 1`).Scan(&projectID); err != nil {
		b.Fatalf("read project ID: %v", err)
	}
	message := strings.Repeat("m", 2048)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatalf("BeginTx() error = %v", err)
	}
	for i := 0; i < entries; i++ {
		id := fmt.Sprintf("benchmark-%05d", i)
		if _, err := tx.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES (?, ?, 'discover', 'benchmark', ?, '2026-07-10T00:00:00Z', '2026-07-10T00:00:00Z')`, id, projectID, message); err != nil {
			tx.Rollback()
			b.Fatalf("insert benchmark journal row %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("Commit() error = %v", err)
	}
	tx, err = store.db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatalf("BeginTx(rebuild) error = %v", err)
	}
	if _, err := rebuildJournalSearchTx(ctx, tx); err != nil {
		tx.Rollback()
		b.Fatalf("rebuildJournalSearchTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		b.Fatalf("Commit(rebuild) error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parity, err := InspectJournalSearchParity(ctx, store)
		if err != nil {
			b.Fatalf("InspectJournalSearchParity() error = %v", err)
		}
		if !parity.Ready {
			b.Fatalf("benchmark parity = %#v, want ready", parity)
		}
	}
}

func openTestStoreBenchmark(b *testing.B, root project.Root, stateHome string) *Store {
	b.Helper()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		b.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		b.Fatalf("OpenStore() error = %v", err)
	}
	return store
}
