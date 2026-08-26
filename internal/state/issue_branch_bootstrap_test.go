package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestBootstrapIssueBranchContractCreatesBranchAuthority(t *testing.T) {
	ctx := context.Background()
	root, resolver := bootstrapFixture(t)
	created, err := state.CreateIssue(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Bootstrap me",
		Body:  "Problem body.",
		Kind:  state.IssueKindDelivery,
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	result, err := state.BootstrapIssueBranchContract(ctx, root, resolver, state.BootstrapIssueBranchContractOptions{
		IssueID:         created.ID,
		Branch:          "issue/loaf-bootstrap",
		StartedWorktree: filepath.Join(filepath.Dir(root.Path()), "wt-bootstrap"),
	})
	if err != nil {
		t.Fatalf("BootstrapIssueBranchContract() error = %v", err)
	}
	if !result.Created {
		t.Fatal("Created = false, want true on first bootstrap")
	}
	if result.AuthorityRef.String() != "branch:issue/loaf-bootstrap" {
		t.Fatalf("authority ref = %q", result.AuthorityRef.String())
	}

	shown, err := state.ShowWorkContract(ctx, root, resolver, result.AuthorityRef.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if shown.Contract.StartedBranch != "issue/loaf-bootstrap" {
		t.Fatalf("StartedBranch = %q", shown.Contract.StartedBranch)
	}

	second, err := state.BootstrapIssueBranchContract(ctx, root, resolver, state.BootstrapIssueBranchContractOptions{
		IssueID: created.ID,
		Branch:  "issue/loaf-bootstrap",
	})
	if err != nil {
		t.Fatalf("second BootstrapIssueBranchContract() error = %v", err)
	}
	if second.Created {
		t.Fatal("second Created = true, want idempotent refresh")
	}
}

func bootstrapFixture(t *testing.T) (project.Root, state.PathResolver) {
	t.Helper()
	dir := t.TempDir()
	agents := filepath.Join(dir, ".agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agents, "loaf.json"), []byte(`{"issue":{"prefix":"LOAF"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	resolver := state.PathResolver{StateHome: t.TempDir()}
	if _, err := state.Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return root, resolver
}
