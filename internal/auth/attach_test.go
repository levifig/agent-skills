package auth_test

import (
	"context"
	"fmt"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/internal/syncserver"
)

func TestUnattendedAttachRoundTrip(t *testing.T) {
	ctx := context.Background()
	h := newAttachHarness(t)

	confDir := filepath.Join(h.repoRoot, ".agents")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	confID := "conf_attach_test"
	if err := h.projectStore.RegisterConfLabel(ctx, confID, h.projectID); err != nil {
		t.Fatal(err)
	}
	conf := map[string]string{"conf_id": confID, "project_id": h.projectID}
	rawConf, _ := json.Marshal(conf)
	if err := os.WriteFile(filepath.Join(confDir, "loaf.conf"), rawConf, 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := project.ResolveRoot(h.repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := crypto.EncodeBundledClientCredential(h.credential)
	if err != nil {
		t.Fatal(err)
	}

	authStore := auth.NewStore(filepath.Join(h.stateHome, "auth"))
	if err := os.MkdirAll(authStore.Dir, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := auth.UnattendedAttach(ctx, auth.AttachInput{
		Root:              root,
		Store:             authStore,
		ClientWire:        wire,
		ConnectionName:    "ci-attach",
		HTTPClient:        h.server.Client(),
		ProbeStore:        h.projectStore,
		AllowInsecureHTTP: true,
	})
	if err != nil {
		t.Fatalf("UnattendedAttach() error = %v", err)
	}
	if result.ProjectID != h.projectID {
		t.Fatalf("project id = %q, want %q", result.ProjectID, h.projectID)
	}
	attached, err := authStore.IsAttached()
	if err != nil || !attached {
		t.Fatalf("IsAttached() = (%v, %v), want (true, nil)", attached, err)
	}
}

func TestUnattendedAttachRefusesIdentityMismatch(t *testing.T) {
	ctx := context.Background()
	h := newAttachHarness(t)
	confDir := filepath.Join(h.repoRoot, ".agents")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rawConf, _ := json.Marshal(map[string]string{"project_id": "proj_other"})
	if err := os.WriteFile(filepath.Join(confDir, "loaf.conf"), rawConf, 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := project.ResolveRoot(h.repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := crypto.EncodeBundledClientCredential(h.credential)
	if err != nil {
		t.Fatal(err)
	}
	authStore := auth.NewStore(filepath.Join(h.stateHome, "auth"))
	if err := os.MkdirAll(authStore.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = auth.UnattendedAttach(ctx, auth.AttachInput{
		Root: root, Store: authStore, ClientWire: wire, HTTPClient: h.server.Client(), ProbeStore: h.projectStore, AllowInsecureHTTP: true,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "identity") && !strings.Contains(strings.ToLower(err.Error()), "match") {
		t.Fatalf("UnattendedAttach() = %v, want identity mismatch refusal", err)
	}
}

type attachHarness struct {
	repoRoot      string
	stateHome     string
	projectID     string
	projectStore  *state.Store
	credential    crypto.BundledClientCredential
	server        *httptest.Server
	serverStore   *syncserver.Store
	projectKey    [32]byte
	keyGen        int
}

func newAttachHarness(t *testing.T) *attachHarness {
	t.Helper()
	ctx := context.Background()
	repoRoot := t.TempDir()

	serverStore, err := syncserver.OpenStore(filepath.Join(t.TempDir(), "relay.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverStore.Close() })

	projectID := "proj_" + strings.ReplaceAll(strings.TrimPrefix(t.Name(), "Test"), "/", "_")
	adminSecret := "admin-secret"
	_, adminKey, err := serverStore.CreateAccount(ctx, adminSecret)
	if err != nil {
		t.Fatal(err)
	}
	token, tokenSecret, err := serverStore.MintConnectionToken(ctx, adminKey, adminSecret, "ci", projectID)
	if err != nil {
		t.Fatal(err)
	}
	master, err := crypto.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	ring := crypto.NewProjectKeyRing(master, projectID)
	projectKey, err := ring.WriteKey()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(syncserver.NewServer(serverStore).Handler())
	t.Cleanup(server.Close)

	stateHome := t.TempDir()
	isolatedDB := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", isolatedDB)
	resolver := state.PathResolver{StateHome: stateHome}
	root, err := project.ResolveRoot(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	dbPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := state.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.DB().ExecContext(ctx, `INSERT INTO projects (id, identity_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`, projectID, projectID+"-hash", now, now); err != nil {
		t.Fatal(err)
	}

	return &attachHarness{
		repoRoot:     repoRoot,
		stateHome:    stateHome,
		projectID:    projectID,
		projectStore: store,
		credential: crypto.BundledClientCredential{
			Endpoint: server.URL, ConnectionToken: token.TokenID + ":" + tokenSecret,
			ProjectID: projectID, KeyGeneration: ring.WriteGeneration, ProjectKey: projectKey[:],
		},
		server: server, serverStore: serverStore, projectKey: projectKey, keyGen: ring.WriteGeneration,
	}
}

func TestUnattendedAttachRefusesHLCSkew(t *testing.T) {
	ctx := context.Background()
	h := newAttachHarness(t)
	confDir := filepath.Join(h.repoRoot, ".agents")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	confID := "conf_hlc_skew"
	if err := h.projectStore.RegisterConfLabel(ctx, confID, h.projectID); err != nil {
		t.Fatal(err)
	}
	rawConf, _ := json.Marshal(map[string]string{"conf_id": confID, "project_id": h.projectID})
	if err := os.WriteFile(filepath.Join(confDir, "loaf.conf"), rawConf, 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := project.ResolveRoot(h.repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := crypto.EncodeBundledClientCredential(h.credential)
	if err != nil {
		t.Fatal(err)
	}
	authStore := auth.NewStore(filepath.Join(h.stateHome, "auth"))
	if err := os.MkdirAll(authStore.Dir, 0o700); err != nil {
		t.Fatal(err)
	}

	farFuture := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	fact := state.FactEnvelope{
		ID: "018f5c2a-0000-7000-8000-000000000099", ProjectID: h.projectID,
		Kind: state.FactKindJournal,
		Payload: `{"entry_type":"discover","message":"future","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z"}`,
		EnvID: "skew-env", Seq: 1, HLC: fmt.Sprintf("%020d:%06d", farFuture.UnixMilli(), 0), EnvelopeV: 1,
	}
	sealed, err := sealAttachFact(h, fact)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := h.serverStore.PushFacts(ctx, h.projectID, []syncserver.PushInput{{FactID: fact.ID, Blob: sealed}}); err != nil {
		t.Fatalf("push relay: %v", err)
	}
	baselineNow := time.Now().UTC()
	baselineHLC := fmt.Sprintf("%020d:%06d", baselineNow.UnixMilli(), 0)
	if _, err := state.ReceiveFact(ctx, h.projectStore, state.FactEnvelope{
		ID: "018f5c2a-0000-7000-8000-000000000010", ProjectID: h.projectID,
		Kind: state.FactKindJournal,
		Payload: fmt.Sprintf(`{"entry_type":"discover","message":"baseline","created_at":%q,"updated_at":%q}`, baselineNow.Format(time.RFC3339), baselineNow.Format(time.RFC3339)),
		EnvID: "receiver-env", Seq: 1, HLC: baselineHLC, EnvelopeV: 1,
	}, state.ReceiveFactOptions{}); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	_, err = auth.UnattendedAttach(ctx, auth.AttachInput{
		Root: root, Store: authStore, ClientWire: wire, HTTPClient: h.server.Client(),
		ProbeStore: h.projectStore, AllowInsecureHTTP: true, MaxHLCSkewMS: 60_000,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hlc skew") {
		t.Fatalf("UnattendedAttach() = %v, want hlc skew refusal", err)
	}
}

func sealAttachFact(h *attachHarness, fact state.FactEnvelope) ([]byte, error) {
	env, err := crypto.EncryptFactEnvelope(h.projectKey, fact.ID, crypto.FactPlaintext{
		Kind: fact.Kind, Payload: fact.Payload, EnvID: fact.EnvID, Seq: fact.Seq,
		HLC: fact.HLC, EnvelopeV: fact.EnvelopeV, KeyGen: h.keyGen,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

