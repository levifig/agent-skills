package state

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestMoveProjectDoesNotStealTargetClaimedAfterPreflight(t *testing.T) {
	ctx := context.Background()
	rootA := projectRoot(t)
	rootB, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(B) error = %v", err)
	}
	targetRoot, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(target) error = %v", err)
	}
	status, err := Initialize(ctx, rootA, PathResolver{StateHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	mover, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(mover) error = %v", err)
	}
	defer mover.Close()
	claimant, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(claimant) error = %v", err)
	}
	defer claimant.Close()

	identityA, err := mover.LookupProjectIdentityForRoot(ctx, rootA)
	if err != nil {
		t.Fatalf("LookupProjectIdentityForRoot(A) error = %v", err)
	}
	identityB, err := claimant.EnsureProject(ctx, rootB)
	if err != nil {
		t.Fatalf("EnsureProject(B) error = %v", err)
	}
	journalA, err := mover.LogJournal(ctx, rootA, JournalLogOptions{Entry: "decision(move-race): preserve A"})
	if err != nil {
		t.Fatalf("LogJournal(A) error = %v", err)
	}
	journalB, err := claimant.LogJournal(ctx, rootB, JournalLogOptions{Entry: "decision(move-race): preserve B"})
	if err != nil {
		t.Fatalf("LogJournal(B) error = %v", err)
	}

	preflightDone := make(chan struct{})
	allowTransaction := make(chan struct{})
	moveAResult := make(chan error, 1)
	go func() {
		_, moveErr := mover.moveProject(ctx, targetRoot, rootA.Path(), targetRoot.Path(), false, func() {
			close(preflightDone)
			<-allowTransaction
		})
		moveAResult <- moveErr
	}()
	<-preflightDone

	movedB, err := claimant.MoveProject(ctx, targetRoot, rootB.Path(), targetRoot.Path())
	if err != nil {
		close(allowTransaction)
		t.Fatalf("MoveProject(B) error = %v", err)
	}
	close(allowTransaction)
	if err := <-moveAResult; err == nil {
		t.Fatal("MoveProject(A) error = nil, want target ownership conflict")
	} else if !strings.Contains(err.Error(), "target path "+targetRoot.Path()+" is already registered to project "+identityB.ID) {
		t.Fatalf("MoveProject(A) error = %v, want visible target ownership conflict", err)
	}

	if movedB.Project.ID != identityB.ID {
		t.Fatalf("MoveProject(B) project ID = %q, want %q", movedB.Project.ID, identityB.ID)
	}
	assertProjectMoveRaceState(t, mover, identityA, identityB, rootA, rootB, targetRoot, journalA.ID, journalB.ID)

	retriedB, err := claimant.MoveProject(ctx, targetRoot, rootB.Path(), targetRoot.Path())
	if err != nil {
		t.Fatalf("MoveProject(B retry) error = %v", err)
	}
	if retriedB.Project.ID != identityB.ID || retriedB.Project.CurrentPath != targetRoot.Path() {
		t.Fatalf("MoveProject(B retry) project = %#v, want B at target", retriedB.Project)
	}
	assertProjectMoveRaceState(t, mover, identityA, identityB, rootA, rootB, targetRoot, journalA.ID, journalB.ID)
}

func assertProjectMoveRaceState(t *testing.T, store *Store, identityA, identityB ProjectIdentity, rootA, rootB, targetRoot project.Root, journalAID, journalBID string) {
	t.Helper()
	ctx := context.Background()

	var targetOwner string
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM project_paths WHERE path = ?`, targetRoot.Path()).Scan(&targetOwner); err != nil {
		t.Fatalf("read target owner error = %v", err)
	}
	if targetOwner != identityB.ID {
		t.Fatalf("target owner = %q, want B %q", targetOwner, identityB.ID)
	}
	if got := countProjectMoveRaceRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE path = ?`, targetRoot.Path()); got != 1 {
		t.Fatalf("target mappings = %d, want exactly 1", got)
	}
	if got := countCurrentProjectPaths(t, store, identityA.ID); got != 1 {
		t.Fatalf("A current mappings = %d, want 1", got)
	}
	if got := countCurrentProjectPaths(t, store, identityB.ID); got != 1 {
		t.Fatalf("B current mappings = %d, want 1", got)
	}

	var sourceAOwner, sourceBOwner string
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM project_paths WHERE path = ?`, rootA.Path()).Scan(&sourceAOwner); err != nil {
		t.Fatalf("read A source owner error = %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM project_paths WHERE path = ?`, rootB.Path()).Scan(&sourceBOwner); err != nil {
		t.Fatalf("read B source owner error = %v", err)
	}
	if sourceAOwner != identityA.ID || sourceBOwner != identityB.ID {
		t.Fatalf("source owners = A:%q B:%q, want A:%q B:%q", sourceAOwner, sourceBOwner, identityA.ID, identityB.ID)
	}

	var currentPathA, currentPathB string
	if err := store.db.QueryRowContext(ctx, `SELECT current_path FROM projects WHERE id = ?`, identityA.ID).Scan(&currentPathA); err != nil {
		t.Fatalf("read A project error = %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT current_path FROM projects WHERE id = ?`, identityB.ID).Scan(&currentPathB); err != nil {
		t.Fatalf("read B project error = %v", err)
	}
	if currentPathA != rootA.Path() || currentPathB != targetRoot.Path() {
		t.Fatalf("project current paths = A:%q B:%q, want A:%q B:%q", currentPathA, currentPathB, rootA.Path(), targetRoot.Path())
	}

	if got := countProjectMoveRaceRows(t, store, `SELECT COUNT(*) FROM projects WHERE id IN (?, ?)`, identityA.ID, identityB.ID); got != 2 {
		t.Fatalf("preserved projects = %d, want 2", got)
	}
	if got := countProjectMoveRaceRows(t, store, `
SELECT COUNT(*)
FROM journal_entries
WHERE (id = ? AND project_id = ?)
   OR (id = ? AND project_id = ?)
`, journalAID, identityA.ID, journalBID, identityB.ID); got != 2 {
		t.Fatalf("preserved journal entries = %d, want 2", got)
	}
}

func countProjectMoveRaceRows(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count project move race rows error = %v", err)
	}
	return count
}
