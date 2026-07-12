package state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	sqlite3 "github.com/ncruces/go-sqlite3"

	"github.com/levifig/loaf/internal/project"
)

const (
	journalDurabilityChildEnv   = "LOAF_JOURNAL_DURABILITY_CHILD"
	journalDurabilityEnteredEnv = "LOAF_JOURNAL_DURABILITY_ENTERED"
	journalDurabilityReleaseEnv = "LOAF_JOURNAL_DURABILITY_RELEASE"
	journalDurabilityReturnEnv  = "LOAF_JOURNAL_DURABILITY_EXPECT_RETURN"
	journalDurabilityFunction   = "loaf_test_block_journal_insert"
	journalDurabilityTrigger    = "loaf_test_block_journal_insert_trigger"
	journalInterruptedMessage   = "interrupted-row"
	journalDurabilityBaseline   = 10
)

func TestJournalDurabilityAfterInsertTriggerControl(t *testing.T) {
	ctx := context.Background()
	rootPath := projectRoot(t).Path()
	resolvedRootPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		t.Fatalf("resolve test root symlinks: %v", err)
	}
	root, err := project.ResolveRoot(resolvedRootPath)
	if err != nil {
		t.Fatalf("resolve test root: %v", err)
	}
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", databasePath)
	if _, err := Initialize(ctx, root, PathResolver{}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	barrierDir := t.TempDir()
	enteredPath := filepath.Join(barrierDir, "inside-after-insert")
	releasePath := filepath.Join(barrierDir, "release")
	child := exec.Command(os.Args[0], "-test.run=^TestJournalDurabilitySIGKILLChild$", "-test.v")
	child.Dir = root.Path()
	child.Env = append(os.Environ(),
		journalDurabilityChildEnv+"=1",
		journalDurabilityEnteredEnv+"="+enteredPath,
		journalDurabilityReleaseEnv+"="+releasePath,
		journalDurabilityReturnEnv+"=1",
	)
	var childOutput bytes.Buffer
	child.Stdout = &childOutput
	child.Stderr = &childOutput
	if err := child.Start(); err != nil {
		t.Fatalf("start durability control child: %v", err)
	}
	t.Cleanup(func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
	})
	waitDone := make(chan error, 1)
	go func() { waitDone <- child.Wait() }()
	waitForJournalDurabilitySignal(t, enteredPath, waitDone, &childOutput)
	if err := os.WriteFile(releasePath, []byte("return\n"), 0o600); err != nil {
		t.Fatalf("release durability control callback: %v", err)
	}
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("durability control child error = %v\n%s", err, childOutput.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("durability control child did not exit after callback release")
	}
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		t.Fatalf("open durability control database: %v", err)
	}
	defer store.Close()
	var canonical, derived int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE message = ?`, journalInterruptedMessage).Scan(&canonical); err != nil {
		t.Fatalf("count control canonical rows: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE message = ?`, journalInterruptedMessage).Scan(&derived); err != nil {
		t.Fatalf("count control derived rows: %v", err)
	}
	if canonical != 1 || derived != 1 {
		t.Fatalf("control rows canonical=%d derived=%d, want one/one", canonical, derived)
	}
	parity, err := inspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("inspect durability control parity: %v", err)
	}
	if parity != (JournalSearchParity{CanonicalRows: 1, IndexRows: 1, Ready: true}) {
		t.Fatalf("control parity=%#v, want one exact ready row", parity)
	}
}

// TestJournalDurabilitySIGKILL proves transaction/process atomicity only: a
// writer killed from inside an AFTER INSERT trigger leaves the interrupted
// canonical/index pair absent while earlier commits remain intact. It
// deliberately makes no OS-crash or power-loss claim.
func TestJournalDurabilitySIGKILL(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("SIGKILL semantics are unavailable on this platform")
	}
	ctx := context.Background()
	for iteration := 0; iteration < 5; iteration++ {
		rootPath := projectRoot(t).Path()
		resolvedRootPath, err := filepath.EvalSymlinks(rootPath)
		if err != nil {
			t.Fatalf("iteration %d resolve test root symlinks: %v", iteration, err)
		}
		root, err := project.ResolveRoot(resolvedRootPath)
		if err != nil {
			t.Fatalf("iteration %d resolve test root: %v", iteration, err)
		}
		databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
		t.Setenv("LOAF_DB", databasePath)
		if _, err := Initialize(ctx, root, PathResolver{}); err != nil {
			t.Fatalf("iteration %d Initialize() error = %v", iteration, err)
		}
		store, err := OpenStore(databasePath)
		if err != nil {
			t.Fatalf("iteration %d OpenStore() error = %v", iteration, err)
		}
		for sequence := 0; sequence < journalDurabilityBaseline; sequence++ {
			if _, err := store.LogJournal(ctx, root, JournalLogOptions{
				Entry: fmt.Sprintf("decision(sigkill-baseline): baseline-%02d", sequence),
			}); err != nil {
				store.Close()
				t.Fatalf("iteration %d baseline LogJournal(%d): %v", iteration, sequence, err)
			}
		}
		if err := store.Close(); err != nil {
			t.Fatalf("iteration %d close baseline store: %v", iteration, err)
		}

		enteredPath := filepath.Join(t.TempDir(), "inside-before-insert")
		child := exec.Command(os.Args[0], "-test.run=^TestJournalDurabilitySIGKILLChild$", "-test.v")
		child.Dir = root.Path()
		child.Env = append(os.Environ(),
			journalDurabilityChildEnv+"=1",
			journalDurabilityEnteredEnv+"="+enteredPath,
		)
		var childOutput bytes.Buffer
		child.Stdout = &childOutput
		child.Stderr = &childOutput
		if err := child.Start(); err != nil {
			t.Fatalf("iteration %d start durability child: %v", iteration, err)
		}
		t.Cleanup(func() {
			if child.Process != nil {
				_ = child.Process.Kill()
			}
		})
		waitDone := make(chan error, 1)
		go func() { waitDone <- child.Wait() }()

		waitForJournalDurabilitySignal(t, enteredPath, waitDone, &childOutput)

		if err := child.Process.Kill(); err != nil {
			t.Fatalf("iteration %d SIGKILL child inside journal transaction: %v", iteration, err)
		}
		select {
		case <-time.After(5 * time.Second):
			t.Fatalf("iteration %d wait for killed child timed out", iteration)
		case err := <-waitDone:
			if err == nil {
				t.Fatalf("iteration %d killed child exited successfully, want signal termination", iteration)
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("iteration %d killed child error = %T %v, want process exit error", iteration, err, err)
			}
		}

		store, err = OpenStoreReadOnly(databasePath)
		if err != nil {
			t.Fatalf("iteration %d reopen read-only state: %v", iteration, err)
		}
		var canonical, derived, interruptedCanonical, interruptedDerived int
		queries := []struct {
			destination *int
			query       string
			args        []any
		}{
			{&canonical, `SELECT COUNT(*) FROM journal_entries`, nil},
			{&derived, `SELECT COUNT(*) FROM journal_search`, nil},
			{&interruptedCanonical, `SELECT COUNT(*) FROM journal_entries WHERE message = ?`, []any{journalInterruptedMessage}},
			{&interruptedDerived, `SELECT COUNT(*) FROM journal_search WHERE message = ?`, []any{journalInterruptedMessage}},
		}
		for _, query := range queries {
			if err := store.db.QueryRowContext(ctx, query.query, query.args...).Scan(query.destination); err != nil {
				store.Close()
				t.Fatalf("iteration %d query %q: %v", iteration, query.query, err)
			}
		}
		if canonical != journalDurabilityBaseline || derived != journalDurabilityBaseline {
			store.Close()
			t.Fatalf("iteration %d rows canonical=%d derived=%d, want baseline %d/%d", iteration, canonical, derived, journalDurabilityBaseline, journalDurabilityBaseline)
		}
		if interruptedCanonical != 0 || interruptedDerived != 0 {
			store.Close()
			t.Fatalf("iteration %d interrupted rows canonical=%d derived=%d, want zero/zero", iteration, interruptedCanonical, interruptedDerived)
		}
		var quickCheck string
		if err := store.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&quickCheck); err != nil {
			store.Close()
			t.Fatalf("iteration %d quick_check: %v", iteration, err)
		}
		if quickCheck != "ok" {
			store.Close()
			t.Fatalf("iteration %d quick_check=%q, want ok", iteration, quickCheck)
		}
		rows, err := store.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			store.Close()
			t.Fatalf("iteration %d foreign_key_check: %v", iteration, err)
		}
		if rows.Next() {
			rows.Close()
			store.Close()
			t.Fatalf("iteration %d foreign_key_check returned a violation", iteration)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			store.Close()
			t.Fatalf("iteration %d iterate foreign_key_check: %v", iteration, err)
		}
		rows.Close()
		parity, err := inspectJournalSearchParity(ctx, store)
		if closeErr := store.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			t.Fatalf("iteration %d inspect journal parity: %v", iteration, err)
		}
		if parity != (JournalSearchParity{CanonicalRows: journalDurabilityBaseline, IndexRows: journalDurabilityBaseline, Ready: true}) {
			t.Fatalf("iteration %d parity=%#v, want exact ready baseline", iteration, parity)
		}
	}
}

// TestJournalDurabilitySIGKILLChild installs a test-only blocking scalar
// function and trigger on the production Store connection. The parent kills
// this process only after the AFTER INSERT callback publishes its signal.
func TestJournalDurabilitySIGKILLChild(t *testing.T) {
	if os.Getenv(journalDurabilityChildEnv) != "1" {
		return
	}
	root, err := project.ResolveRoot("")
	if err != nil {
		t.Fatalf("resolve child project root: %v", err)
	}
	databasePath, err := (PathResolver{}).DatabasePath(root)
	if err != nil {
		t.Fatalf("resolve child database path: %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("open child store: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	connection, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire child SQL connection: %v", err)
	}
	enteredPath := os.Getenv(journalDurabilityEnteredEnv)
	releasePath := os.Getenv(journalDurabilityReleaseEnv)
	err = connection.Raw(func(driverConnection any) error {
		rawConnection, ok := driverConnection.(interface{ Raw() *sqlite3.Conn })
		if !ok {
			return fmt.Errorf("driver connection %T does not expose Raw", driverConnection)
		}
		return rawConnection.Raw().CreateFunction(journalDurabilityFunction, 0, sqlite3.INNOCUOUS, func(sqlContext sqlite3.Context, _ ...sqlite3.Value) {
			temporaryPath := enteredPath + ".tmp"
			if err := os.WriteFile(temporaryPath, []byte("inside-after-insert\n"), 0o600); err != nil {
				sqlContext.ResultError(err)
				return
			}
			if err := os.Rename(temporaryPath, enteredPath); err != nil {
				sqlContext.ResultError(err)
				return
			}
			for {
				if releasePath != "" {
					if _, err := os.Stat(releasePath); err == nil {
						sqlContext.ResultInt(1)
						return
					} else if !os.IsNotExist(err) {
						sqlContext.ResultError(err)
						return
					}
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	})
	if err != nil {
		connection.Close()
		t.Fatalf("register blocking SQLite function: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("return child SQL connection: %v", err)
	}
	triggerSQL := fmt.Sprintf(`
CREATE TRIGGER %s
AFTER INSERT ON journal_entries
WHEN NEW.message = '%s'
BEGIN
  SELECT %s();
END`, journalDurabilityTrigger, journalInterruptedMessage, journalDurabilityFunction)
	if _, err := store.db.ExecContext(ctx, triggerSQL); err != nil {
		t.Fatalf("install blocking journal trigger: %v", err)
	}
	if _, err := store.LogJournal(ctx, root, JournalLogOptions{
		Entry: "decision(sigkill): " + journalInterruptedMessage,
	}); err != nil {
		t.Fatalf("child LogJournal(): %v", err)
	}
	if os.Getenv(journalDurabilityReturnEnv) == "1" {
		return
	}
	t.Fatal("child LogJournal returned after blocking trigger")
}

func waitForJournalDurabilitySignal(t *testing.T, enteredPath string, waitDone <-chan error, childOutput *bytes.Buffer) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-waitDone:
			t.Fatalf("durability child exited before entering trigger: %v\n%s", err, childOutput.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for in-trigger signal\n%s", childOutput.String())
		case <-ticker.C:
			if _, err := os.Stat(enteredPath); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatalf("inspect in-trigger signal: %v", err)
			}
		}
	}
}
