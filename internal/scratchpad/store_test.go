package scratchpad_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/scratchpad"
	"github.com/levifig/loaf/internal/state"
)

func TestScratchpadKindsUseScratchpadPermanence(t *testing.T) {
	for _, kind := range []string{
		scratchpad.KindIntro,
		scratchpad.KindMessage,
		scratchpad.KindClaim,
		scratchpad.KindRelease,
	} {
		permanence, ok := state.FactPermanenceClass(kind)
		if !ok {
			t.Fatalf("kind %q not registered", kind)
		}
		if permanence != state.PermanenceScratchpad {
			t.Fatalf("kind %q permanence = %q, want %q", kind, permanence, state.PermanenceScratchpad)
		}
	}
}

func TestAppendReadClaimReleaseDoesNotWriteJournal(t *testing.T) {
	ctx := context.Background()
	root, resolver := testProject(t)

	if _, err := scratchpad.AppendMessage(ctx, root, resolver, scratchpad.AppendOptions{
		Channel:    "effort-1",
		InstanceID: "agent-a",
		Who:        "Agent A",
		Text:       "checking in",
	}); err != nil {
		t.Fatalf("AppendMessage() error = %v", err)
	}
	if _, err := scratchpad.Claim(ctx, root, resolver, scratchpad.ClaimOptions{
		Channel:    "effort-1",
		InstanceID: "agent-a",
		Resource:   "internal/cli/cli.go",
		TTL:        time.Minute,
	}); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := scratchpad.Release(ctx, root, resolver, scratchpad.ReleaseOptions{
		Channel:    "effort-1",
		InstanceID: "agent-a",
		Resource:   "internal/cli/cli.go",
	}); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	view, err := scratchpad.ReadChannel(ctx, root, resolver, "effort-1", 0)
	if err != nil {
		t.Fatalf("ReadChannel() error = %v", err)
	}
	if len(view.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(view.Messages))
	}
	if len(view.Roster) != 1 {
		t.Fatalf("roster = %d, want 1", len(view.Roster))
	}
	if len(view.ActiveClaims) != 0 {
		t.Fatalf("active claims = %d, want 0 after release", len(view.ActiveClaims))
	}

	status, err := state.Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := state.OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var journalFacts int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE kind = ?`, state.FactKindJournal).Scan(&journalFacts); err != nil {
		t.Fatalf("count journal facts: %v", err)
	}
	if journalFacts != 0 {
		t.Fatalf("journal facts = %d, want 0", journalFacts)
	}
	var scratchpadFacts int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE kind LIKE 'scratchpad_%'`).Scan(&scratchpadFacts); err != nil {
		t.Fatalf("count scratchpad facts: %v", err)
	}
	if scratchpadFacts < 3 {
		t.Fatalf("scratchpad facts = %d, want at least intro+message+claim+release", scratchpadFacts)
	}
}

func TestExpiredClaimsAreNotActive(t *testing.T) {
	ctx := context.Background()
	root, resolver := testProject(t)
	if _, err := scratchpad.Claim(ctx, root, resolver, scratchpad.ClaimOptions{
		Channel:    "effort-2",
		InstanceID: "agent-b",
		Resource:   "README.md",
		TTL:        time.Millisecond,
	}); err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	view, err := scratchpad.ReadChannel(ctx, root, resolver, "effort-2", 0)
	if err != nil {
		t.Fatalf("ReadChannel() error = %v", err)
	}
	if len(view.ActiveClaims) != 0 {
		t.Fatalf("active claims = %d, want 0 for expired lease", len(view.ActiveClaims))
	}
}

func testProject(t *testing.T) (project.Root, state.PathResolver) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	stateHome := t.TempDir()
	t.Setenv("LOAF_DB", filepath.Join(stateHome, "loaf.sqlite"))
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return root, resolver
}
