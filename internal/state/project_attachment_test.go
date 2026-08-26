package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestResolveProjectByConfUnattendedRefusesUnknownLabel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	root := projectRootInTempDir(t)
	identity, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}
	conf := project.ProjectConf{ConfID: "conf_unknown", ProjectID: identity.ID}
	_, err = store.ResolveProjectByConf(ctx, conf, ConfResolutionUnattended)
	var required *ConfResolutionRequiredError
	if !errors.As(err, &required) {
		t.Fatalf("ResolveProjectByConf() error = %v, want *ConfResolutionRequiredError", err)
	}
}

func TestResolveProjectByConfRefusesOutOfScopeMapping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	first := projectRootInTempDir(t)
	second := projectRootInTempDir(t)
	firstID, err := store.EnsureProject(ctx, first)
	if err != nil {
		t.Fatalf("EnsureProject(first) error = %v", err)
	}
	secondID, err := store.EnsureProject(ctx, second)
	if err != nil {
		t.Fatalf("EnsureProject(second) error = %v", err)
	}
	if err := store.RegisterConfLabel(ctx, "conf_shared", firstID.ID); err != nil {
		t.Fatalf("RegisterConfLabel() error = %v", err)
	}
	conf := project.ProjectConf{ConfID: "conf_shared", ProjectID: secondID.ID}
	_, err = store.ResolveProjectByConf(ctx, conf, ConfResolutionUnattended)
	var outOfScope *ConfOutOfScopeError
	if !errors.As(err, &outOfScope) {
		t.Fatalf("ResolveProjectByConf() error = %v, want *ConfOutOfScopeError", err)
	}
}

func TestResolveProjectByConfSucceedsForRegisteredLabel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	root := projectRootInTempDir(t)
	identity, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}
	if err := store.RegisterConfLabel(ctx, "conf_ok", identity.ID); err != nil {
		t.Fatalf("RegisterConfLabel() error = %v", err)
	}
	got, err := store.ResolveProjectByConf(ctx, project.ProjectConf{ConfID: "conf_ok", ProjectID: identity.ID}, ConfResolutionUnattended)
	if err != nil {
		t.Fatalf("ResolveProjectByConf() error = %v", err)
	}
	if got.ID != identity.ID {
		t.Fatalf("ID = %q, want %q", got.ID, identity.ID)
	}
}

func TestInspectDuplicateUniverseSuspectsFlagsSharedRemote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	first := projectRootInTempDir(t)
	second := projectRootInTempDir(t)
	firstID, err := store.EnsureProject(ctx, first)
	if err != nil {
		t.Fatalf("EnsureProject(first) error = %v", err)
	}
	secondID, err := store.EnsureProject(ctx, second)
	if err != nil {
		t.Fatalf("EnsureProject(second) error = %v", err)
	}
	shared := "github.com/example/shared"
	if err := store.RecordAttachmentEvidence(ctx, firstID.ID, EvidenceKindRemote, shared, ""); err != nil {
		t.Fatalf("RecordAttachmentEvidence(first) error = %v", err)
	}
	if err := store.RecordAttachmentEvidence(ctx, secondID.ID, EvidenceKindRemote, shared, ""); err != nil {
		t.Fatalf("RecordAttachmentEvidence(second) error = %v", err)
	}
	report, err := InspectDuplicateUniverseSuspects(ctx, store)
	if err != nil {
		t.Fatalf("InspectDuplicateUniverseSuspects() error = %v", err)
	}
	if report.Ready {
		t.Fatal("Ready = true, want duplicate remote suspect")
	}
	if len(report.RemoteGroups) != 1 || len(report.RemoteGroups[0].ProjectIDs) != 2 {
		t.Fatalf("RemoteGroups = %#v, want one group with two projects", report.RemoteGroups)
	}
}

func TestWriteProjectConfRoundTrip(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	root, err := project.ResolveRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	want := project.ProjectConf{ProjectID: "proj_test123"}
	if err := project.WriteProjectConf(root, want); err != nil {
		t.Fatalf("WriteProjectConf() error = %v", err)
	}
	got, err := project.ReadProjectConf(root)
	if err != nil {
		t.Fatalf("ReadProjectConf() error = %v", err)
	}
	if got.ProjectID != want.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", got.ProjectID, want.ProjectID)
	}
	if got.ConfID == "" {
		t.Fatal("ConfID is empty, want generated label")
	}
	path := filepath.Join(rootDir, ".agents", "loaf.conf")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat(conf) error = %v", err)
	}
}

func openAttachmentTestStore(t *testing.T) *Store {
	t.Helper()
	stateHome := t.TempDir()
	root := projectRootInTempDir(t)
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}


func TestBootstrapProjectConfSkipsSilentBackfill(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	root := projectRootInTempDir(t)
	identity, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}
	if err := store.RecordAttachmentEvidence(ctx, identity.ID, EvidenceKindPath, root.Path(), ""); err != nil {
		t.Fatalf("RecordAttachmentEvidence() error = %v", err)
	}
	if err := store.BootstrapProjectConf(ctx, root); err != nil {
		t.Fatalf("BootstrapProjectConf() error = %v", err)
	}
	if _, err := project.ReadProjectConf(root); err == nil {
		t.Fatal("ReadProjectConf() succeeded, want missing conf for backfill-skipped project")
	}
}

func TestInspectOperationalInvariantsFlagsDuplicateUniverse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := openAttachmentTestStore(t)
	first := projectRootInTempDir(t)
	second := projectRootInTempDir(t)
	firstID, err := store.EnsureProject(ctx, first)
	if err != nil {
		t.Fatalf("EnsureProject(first) error = %v", err)
	}
	secondID, err := store.EnsureProject(ctx, second)
	if err != nil {
		t.Fatalf("EnsureProject(second) error = %v", err)
	}
	shared := "github.com/example/shared-root"
	if err := store.RecordAttachmentEvidence(ctx, firstID.ID, EvidenceKindRemote, shared, ""); err != nil {
		t.Fatalf("RecordAttachmentEvidence(first) error = %v", err)
	}
	if err := store.RecordAttachmentEvidence(ctx, secondID.ID, EvidenceKindRemote, shared, ""); err != nil {
		t.Fatalf("RecordAttachmentEvidence(second) error = %v", err)
	}
	diagnostics, _, err := inspectOperationalInvariants(ctx, store, InspectOptions{DuplicateUniverse: true})
	if err != nil {
		t.Fatalf("inspectOperationalInvariants() error = %v", err)
	}
	found := false
	for _, d := range diagnostics {
		if d.Code == DuplicateUniverseSuspectCode {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want %s", diagnostics, DuplicateUniverseSuspectCode)
	}
}
