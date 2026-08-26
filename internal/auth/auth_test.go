package auth_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/syncserver"
)

func TestSetupLinkListRevokeRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	stateHome := t.TempDir()
	serverDB := filepath.Join(t.TempDir(), "sync.sqlite")
	projectID := "proj_test_auth_roundtrip"

	setup, err := auth.Setup(ctx, auth.NewStore(stateHome), auth.SetupInput{
		Endpoint: "http://127.0.0.1:8080",
		ServerDB: serverDB,
	})
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if setup.EmergencyKit == "" {
		t.Fatal("Setup() emergency kit is empty")
	}

	link, err := auth.Link(ctx, auth.NewStore(stateHome), auth.LinkInput{
		Name:      "ci-runner",
		ProjectID: projectID,
	})
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if !strings.HasPrefix(link.ClientWire, "loafclient1://") {
		t.Fatalf("client wire = %q, want loafclient1 prefix", link.ClientWire)
	}
	bundle, err := crypto.DecodeBundledClientCredential(link.ClientWire)
	if err != nil {
		t.Fatalf("DecodeBundledClientCredential() error = %v", err)
	}
	if bundle.ProjectID != projectID {
		t.Fatalf("project id = %q, want %q", bundle.ProjectID, projectID)
	}

	rows, err := auth.ListConnections(ctx, auth.NewStore(stateHome), "")
	if err != nil {
		t.Fatalf("ListConnections() error = %v", err)
	}
	if len(rows) != 1 || rows[0].Name != "ci-runner" {
		t.Fatalf("rows = %#v, want one ci-runner token", rows)
	}

	if err := auth.RevokeConnection(ctx, auth.NewStore(stateHome), "ci-runner", ""); err != nil {
		t.Fatalf("RevokeConnection() error = %v", err)
	}
	server, err := syncserver.OpenStore(serverDB)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer server.Close()
	if err := server.AuthenticateConnectionToken(ctx, link.ConnectionTokenID, strings.SplitN(bundle.ConnectionToken, ":", 2)[1], projectID); err == nil {
		t.Fatal("revoked token still authenticates")
	}
}

func TestCommandRequiresAttach(t *testing.T) {
	t.Parallel()
	if !auth.CommandRequiresAttach([]string{"journal", "recent"}) {
		t.Fatal("journal should require attach")
	}
	if auth.CommandRequiresAttach([]string{"auth", "setup"}) {
		t.Fatal("auth should be exempt")
	}
	if auth.CommandRequiresAttach([]string{"state", "path"}) {
		t.Fatal("state path should be exempt")
	}
}



func TestEnforcementInactiveUntilSetup(t *testing.T) {
	t.Parallel()
	store := auth.NewStore(t.TempDir())
	active, err := store.EnforcementActive()
	if err != nil || active {
		t.Fatalf("EnforcementActive() = (%v, %v), want (false, nil) before setup", active, err)
	}
}
