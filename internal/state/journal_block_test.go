package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestJournalBlockUnblockWriteInvariant(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	for _, entry := range []string{"block: missing key", "block(   ): blank key", "unblock: missing key", "unblock(   ): blank key"} {
		if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: entry}); err == nil || !strings.Contains(err.Error(), "require a nonempty key") {
			t.Fatalf("LogJournal(%q) error = %v, want nonempty-key validation", entry, err)
		}
	}

	assertUnmatched := func(entry, key, latestID, latestType string) {
		t.Helper()
		_, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: entry})
		var unmatched *JournalUnmatchedUnblockError
		if !errors.As(err, &unmatched) {
			t.Fatalf("LogJournal(%q) error = %T %v, want JournalUnmatchedUnblockError", entry, err, err)
		}
		if unmatched.Code != JournalUnmatchedUnblockCode || unmatched.Key != key || unmatched.LatestSourceID != latestID || unmatched.LatestSourceType != latestType {
			t.Fatalf("LogJournal(%q) error = %#v, want code/key/latest %q/%q/%q", entry, unmatched, key, latestID, latestType)
		}
	}

	assertUnmatched("unblock(CaseKey): never matched", "CaseKey", "", "")
	block, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "block(CaseKey): opened"})
	if err != nil {
		t.Fatalf("block error = %v", err)
	}
	assertUnmatched("unblock(casekey): exact case required", "casekey", "", "")
	unblock, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "unblock(CaseKey): resolved"})
	if err != nil {
		t.Fatalf("matched unblock error = %v", err)
	}
	assertUnmatched("unblock(CaseKey): duplicate", "CaseKey", unblock.ID, "unblock")
	reopened, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "block(CaseKey): reopened"})
	if err != nil {
		t.Fatalf("reopen block error = %v", err)
	}
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "unblock(CaseKey): resolved again"}); err != nil {
		t.Fatalf("unblock after reopen error = %v", err)
	}
	if block.ID == reopened.ID {
		t.Fatalf("reopened block ID = original ID %q, want distinct writes", block.ID)
	}

	var canonical, derived int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE entry_type IN ('block', 'unblock')`).Scan(&canonical); err != nil {
		t.Fatalf("count canonical: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE entry_type IN ('block', 'unblock')`).Scan(&derived); err != nil {
		t.Fatalf("count derived: %v", err)
	}
	if canonical != 4 || derived != canonical {
		t.Fatalf("blocker rows canonical=%d derived=%d, want 4/4 (rejections write nothing)", canonical, derived)
	}
}

func TestJournalUnblockIsProjectScopedAndPreservesHistoricalBadRows(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	if _, err := Initialize(ctx, rootA, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	if _, err := Initialize(ctx, rootB, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(B) error = %v", err)
	}
	store := openTestStore(t, rootA, stateHome)
	defer store.Close()
	projectA := projectIDForTest(t, store, rootA)
	now := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES ('historical-bad-unblock', ?, 'unblock', 'shared', 'legacy bad row', ?, ?)`, projectA, now, now); err != nil {
		t.Fatalf("insert historical bad row: %v", err)
	}

	if _, err := store.LogJournal(ctx, rootB, JournalLogOptions{Entry: "block(shared): project B open"}); err != nil {
		t.Fatalf("block(B) error = %v", err)
	}
	_, err := store.LogJournal(ctx, rootA, JournalLogOptions{Entry: "unblock(shared): project B must not match"})
	var unmatched *JournalUnmatchedUnblockError
	if !errors.As(err, &unmatched) || unmatched.LatestSourceID != "historical-bad-unblock" {
		t.Fatalf("unblock(A) error = %#v, want project-A historical latest outcome", err)
	}
	if _, err := store.LogJournal(ctx, rootA, JournalLogOptions{Entry: "block(shared): reopen after historical bad row"}); err != nil {
		t.Fatalf("block(A) error = %v", err)
	}
	if _, err := store.LogJournal(ctx, rootA, JournalLogOptions{Entry: "unblock(shared): project A resolved"}); err != nil {
		t.Fatalf("unblock(A) error = %v", err)
	}
	if _, err := store.LogJournal(ctx, rootB, JournalLogOptions{Entry: "unblock(shared): project B resolved"}); err != nil {
		t.Fatalf("unblock(B) error = %v", err)
	}
	var historical int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = 'historical-bad-unblock'`).Scan(&historical); err != nil || historical != 1 {
		t.Fatalf("historical row count=%d error=%v, want untouched", historical, err)
	}
}

func TestJournalUnblockLatestOutcomeUsesCreatedAtThenRowID(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	createdAt := "2026-07-11T12:00:00Z"
	for _, row := range []struct {
		id        string
		entryType string
		message   string
	}{
		{id: "tie-block", entryType: "block", message: "first at tied timestamp"},
		{id: "tie-unblock", entryType: "unblock", message: "later rowid at tied timestamp"},
	} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES (?, ?, ?, 'tie-key', ?, ?, ?)`, row.id, projectID, row.entryType, row.message, createdAt, createdAt); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
	_, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "unblock(tie-key): must see tied later rowid"})
	var unmatched *JournalUnmatchedUnblockError
	if !errors.As(err, &unmatched) || unmatched.LatestSourceID != "tie-unblock" || unmatched.LatestSourceType != "unblock" {
		t.Fatalf("tied latest error = %#v, want tie-unblock latest outcome", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES ('tie-reopen', ?, 'block', 'tie-key', 'highest rowid reopens', ?, ?)`, projectID, createdAt, createdAt); err != nil {
		t.Fatalf("insert tied reopening block: %v", err)
	}
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "unblock(tie-key): tied rowid block matched"}); err != nil {
		t.Fatalf("unblock tied reopening block error = %v", err)
	}
}

func TestJournalLogTransactionFailuresRollbackCanonicalSearchAndOrigin(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "block(rollback): open"}); err != nil {
		t.Fatalf("baseline block error = %v", err)
	}

	stages := []struct {
		name  string
		hooks journalLogHooks
	}{
		{name: "canonical", hooks: journalLogHooks{afterCanonical: func(*sql.Tx) error { return errors.New("forced canonical failure") }}},
		{name: "search", hooks: journalLogHooks{afterSearch: func(*sql.Tx) error { return errors.New("forced search failure") }}},
		{name: "origin", hooks: journalLogHooks{afterOrigin: func(*sql.Tx) error { return errors.New("forced origin failure") }}},
	}
	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			beforeCanonical := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_entries`)
			beforeSearch := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_search`)
			beforeOrigins := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_origins`)
			_, err := store.logJournalWithHooks(ctx, root, JournalLogOptions{
				Entry:  "unblock(rollback): must roll back",
				Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: JournalOriginMechanismManual},
			}, &stage.hooks)
			if err == nil || !strings.Contains(err.Error(), "forced") {
				t.Fatalf("logJournalWithHooks() error = %v, want forced failure", err)
			}
			if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_entries`); got != beforeCanonical {
				t.Fatalf("canonical rows=%d, want rollback to %d", got, beforeCanonical)
			}
			if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_search`); got != beforeSearch {
				t.Fatalf("search rows=%d, want rollback to %d", got, beforeSearch)
			}
			if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_origins`); got != beforeOrigins {
				t.Fatalf("origin rows=%d, want rollback to %d", got, beforeOrigins)
			}
			parity, err := inspectJournalSearchParity(ctx, store)
			if err != nil || !parity.Ready {
				t.Fatalf("parity after rollback = %#v error=%v, want ready", parity, err)
			}
		})
	}
}

func TestJournalConcurrentUnblockIndependentStores(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "block(race): open"}); err != nil {
		t.Fatalf("block error = %v", err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			ready.Done()
			<-start
			_, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: fmt.Sprintf("unblock(race): contender %d", i)})
			results <- err
		}(i)
	}
	ready.Wait()
	close(start)
	var succeeded, rejected int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
			continue
		}
		var unmatched *JournalUnmatchedUnblockError
		if errors.As(err, &unmatched) && unmatched.LatestSourceType == "unblock" {
			rejected++
			continue
		}
		t.Fatalf("contender error = %T %v, want typed rejection", err, err)
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("race results success=%d rejected=%d, want 1/1", succeeded, rejected)
	}
}

const journalUnblockProcessEnv = "LOAF_JOURNAL_UNBLOCK_PROCESS"

func TestJournalConcurrentUnblockIndependentProcesses(t *testing.T) {
	if os.Getenv(journalUnblockProcessEnv) == "1" {
		runJournalUnblockProcessChild(t)
		return
	}
	ctx := context.Background()
	root := projectRoot(t)
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", databasePath)
	if _, err := Initialize(ctx, root, PathResolver{}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := LogJournal(ctx, root, PathResolver{}, JournalLogOptions{Entry: "block(process-race): open"}); err != nil {
		t.Fatalf("block error = %v", err)
	}
	release := filepath.Join(t.TempDir(), "release")
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, 2)
	for i := range commands {
		commands[i] = exec.Command(os.Args[0], "-test.run=^TestJournalConcurrentUnblockIndependentProcesses$")
		commands[i].Env = append(os.Environ(), journalUnblockProcessEnv+"=1", "LOAF_DB="+databasePath, "LOAF_JOURNAL_UNBLOCK_ROOT="+root.Path(), "LOAF_JOURNAL_UNBLOCK_RELEASE="+release, fmt.Sprintf("LOAF_JOURNAL_UNBLOCK_ID=%d", i))
		commands[i].Stdout = &outputs[i]
		commands[i].Stderr = &outputs[i]
		if err := commands[i].Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}
	if err := os.WriteFile(release, []byte("go"), 0o600); err != nil {
		t.Fatalf("release children: %v", err)
	}
	var success, unmatched int
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("child %d error = %v\n%s", i, err, outputs[i].String())
		}
		switch {
		case strings.Contains(outputs[i].String(), "RESULT success"):
			success++
		case strings.Contains(outputs[i].String(), "RESULT unmatched unblock"):
			unmatched++
		default:
			t.Fatalf("child %d output = %q, want classified result", i, outputs[i].String())
		}
	}
	if success != 1 || unmatched != 1 {
		t.Fatalf("process race success=%d unmatched=%d, want 1/1", success, unmatched)
	}
}

func runJournalUnblockProcessChild(t *testing.T) {
	t.Helper()
	for {
		if _, err := os.Stat(os.Getenv("LOAF_JOURNAL_UNBLOCK_RELEASE")); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	root, err := project.ResolveRoot(os.Getenv("LOAF_JOURNAL_UNBLOCK_ROOT"))
	if err != nil {
		t.Fatalf("ResolveRoot(child) error = %v", err)
	}
	_, err = LogJournal(context.Background(), root, PathResolver{}, JournalLogOptions{Entry: "unblock(process-race): contender " + os.Getenv("LOAF_JOURNAL_UNBLOCK_ID")})
	if err == nil {
		fmt.Println("RESULT success")
		return
	}
	var unmatched *JournalUnmatchedUnblockError
	if !errors.As(err, &unmatched) {
		t.Fatalf("child error = %T %v, want typed unmatched", err, err)
	}
	fmt.Printf("RESULT unmatched %s\n", unmatched.LatestSourceType)
}
