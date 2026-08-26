package auth_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/crypto"
)

func TestSetupLinkListRevokeRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dataHome := t.TempDir()
	serverDB := filepath.Join(t.TempDir(), "sync.sqlite")
	cfg := auth.AdminConfig{DataHome: dataHome, ServerDB: serverDB}

	setup, err := auth.SetupAccount(ctx, cfg, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("SetupAccount() error = %v", err)
	}
	if setup.AccessKeyID == "" || setup.EmergencyKit == "" {
		t.Fatalf("setup = %#v, want access key and emergency kit", setup)
	}
	if _, err := crypto.RecoverMasterKeyFromEmergencyKit(setup.EmergencyKit); err != nil {
		t.Fatalf("RecoverMasterKeyFromEmergencyKit() error = %v", err)
	}

	projectID := "proj_auth_test"
	link, err := auth.LinkConnection(ctx, cfg, auth.LinkOptions{Name: "cursor:loaf", ProjectID: projectID})
	if err != nil {
		t.Fatalf("LinkConnection() error = %v", err)
	}
	if !strings.HasPrefix(link.BundledCredential, "loafclient1://") {
		t.Fatalf("bundled credential = %q, want loafclient1 prefix", link.BundledCredential)
	}
	decoded, err := crypto.DecodeBundledClientCredential(link.BundledCredential)
	if err != nil {
		t.Fatalf("DecodeBundledClientCredential() error = %v", err)
	}
	if decoded.ProjectID != projectID {
		t.Fatalf("decoded project id = %q, want %q", decoded.ProjectID, projectID)
	}

	tokens, err := auth.ListConnections(ctx, cfg)
	if err != nil {
		t.Fatalf("ListConnections() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "cursor:loaf" {
		t.Fatalf("tokens = %#v, want one cursor:loaf token", tokens)
	}

	revoked, err := auth.RevokeConnection(ctx, cfg, "cursor:loaf")
	if err != nil {
		t.Fatalf("RevokeConnection() error = %v", err)
	}
	if !revoked {
		t.Fatal("RevokeConnection() = false, want true")
	}
	tokens, err = auth.ListConnections(ctx, cfg)
	if err != nil {
		t.Fatalf("ListConnections() after revoke error = %v", err)
	}
	if tokens[0].RevokedAt == nil {
		t.Fatalf("token = %#v, want revoked_at set", tokens[0])
	}
}

func TestLinkRequiresAdminCredential(t *testing.T) {
	t.Parallel()
	cfg := auth.AdminConfig{DataHome: t.TempDir(), ServerDB: filepath.Join(t.TempDir(), "sync.sqlite")}
	_, err := auth.LinkConnection(context.Background(), cfg, auth.LinkOptions{Name: "x", ProjectID: "proj_x"})
	if err == nil {
		t.Fatal("LinkConnection() error = nil, want missing admin credential")
	}
}
