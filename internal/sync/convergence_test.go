package sync_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/internal/sync"
	"github.com/levifig/loaf/internal/syncserver"
)

func TestTwoClientConvergenceUnion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newConvergenceHarness(t)
	homeA := openIsolatedStore(t, h.projectID)
	homeB := openIsolatedStore(t, h.projectID)

	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	factA, err := state.AppendFact(ctx, homeA, state.AppendFactInput{
		ProjectID: h.projectID, Kind: state.FactKindJournal,
		Payload: `{"entry_type":"discover","message":"from-a","created_at":"2026-08-26T12:00:00Z","updated_at":"2026-08-26T12:00:00Z"}`,
		EnvID:   "env-a", Now: now,
	})
	if err != nil {
		t.Fatalf("append A: %v", err)
	}
	factB, err := state.AppendFact(ctx, homeB, state.AppendFactInput{
		ProjectID: h.projectID, Kind: state.FactKindJournal,
		Payload: `{"entry_type":"discover","message":"from-b","created_at":"2026-08-26T12:00:01Z","updated_at":"2026-08-26T12:00:01Z"}`,
		EnvID:   "env-b", Now: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("append B: %v", err)
	}

	if _, err := newEngine(t, homeA, h).Sync(ctx); err != nil {
		t.Fatalf("sync A push: %v", err)
	}
	if _, err := newEngine(t, homeB, h).Sync(ctx); err != nil {
		t.Fatalf("sync B push: %v", err)
	}
	if _, err := newEngine(t, homeA, h).Sync(ctx); err != nil {
		t.Fatalf("sync A pull: %v", err)
	}
	if _, err := newEngine(t, homeB, h).Sync(ctx); err != nil {
		t.Fatalf("sync B pull: %v", err)
	}

	for name, store := range map[string]*state.Store{"A": homeA, "B": homeB} {
		if _, ok, err := state.GetFactByID(ctx, store, factA.ID); err != nil || !ok {
			t.Fatalf("%s missing factA: ok=%v err=%v", name, ok, err)
		}
		if _, ok, err := state.GetFactByID(ctx, store, factB.ID); err != nil || !ok {
			t.Fatalf("%s missing factB: ok=%v err=%v", name, ok, err)
		}
	}
}

func TestGapInjectionIsLoud(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newConvergenceHarness(t)
	store := openIsolatedStore(t, h.projectID)

	payload, _ := json.Marshal(map[string]string{"env_id": "gap-env"})
	if _, err := state.ReceiveFact(ctx, store, state.FactEnvelope{
		ID: "018f5c2a-0000-7000-8000-000000000099", ProjectID: h.projectID,
		Kind: state.FactKindEnrollment, Payload: string(payload), EnvID: "admin-env", Seq: 1,
		HLC: fmt.Sprintf("%020d:%06d", 1_700_000_000_000, 0), EnvelopeV: 1,
	}, state.ReceiveFactOptions{}); err != nil {
		t.Fatalf("enrollment: %v", err)
	}
	for _, seq := range []int64{1, 3} {
		if _, err := state.ReceiveFact(ctx, store, state.FactEnvelope{
			ID: fmt.Sprintf("018f5c2a-0000-7000-8000-%012d", seq), ProjectID: h.projectID,
			Kind:    state.FactKindJournal,
			Payload: fmt.Sprintf(`{"entry_type":"discover","message":"seq-%d","created_at":"2026-08-26T12:00:00Z","updated_at":"2026-08-26T12:00:00Z"}`, seq),
			EnvID:   "gap-env", Seq: seq, HLC: fmt.Sprintf("%020d:%06d", 1_700_000_000_000+seq, 0), EnvelopeV: 1,
		}, state.ReceiveFactOptions{}); err != nil {
			t.Fatalf("receive seq %d: %v", seq, err)
		}
	}

	gaps, err := sync.DetectEnvSeqGaps(ctx, store, h.projectID)
	if err != nil {
		t.Fatalf("DetectEnvSeqGaps: %v", err)
	}
	if len(gaps) != 1 || gaps[0].Missing[0] != 2 || !gaps[0].Expected {
		t.Fatalf("gaps = %#v, want missing seq 2 enrolled gap-env", gaps)
	}
}

func TestSkewRefusal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newConvergenceHarness(t)
	publisher := openIsolatedStore(t, h.projectID)

	farFuture := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	fact, err := state.AppendFact(ctx, publisher, state.AppendFactInput{
		ProjectID: h.projectID, Kind: state.FactKindJournal,
		Payload: `{"entry_type":"discover","message":"future","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z"}`,
		EnvID:   "skew-env", Now: farFuture,
	})
	if err != nil {
		t.Fatalf("append future: %v", err)
	}
	sealed, err := sealFactForHarness(t, h, fact)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := h.serverStore.PushFacts(ctx, h.projectID, []syncserver.PushInput{{FactID: fact.ID, Blob: sealed}}); err != nil {
		t.Fatalf("push relay: %v", err)
	}

	receiver := openIsolatedStore(t, h.projectID)
	if _, err := state.ReceiveFact(ctx, receiver, state.FactEnvelope{
		ID: "018f5c2a-0000-7000-8000-000000000010", ProjectID: h.projectID,
		Kind:    state.FactKindJournal,
		Payload: `{"entry_type":"discover","message":"baseline","created_at":"2026-08-26T12:00:00Z","updated_at":"2026-08-26T12:00:00Z"}`,
		EnvID:   "receiver-env", Seq: 1, HLC: "00000000000000170000:000000", EnvelopeV: 1,
	}, state.ReceiveFactOptions{}); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	_, err = newEngineWithSkew(t, receiver, h, 60_000).Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "hlc skew") {
		t.Fatalf("Sync() = %v, want hlc skew refusal", err)
	}
}

func TestMisProvisioningCrosscheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newConvergenceHarness(t)
	wrong := h.credential
	wrong.ProjectID = "other-project"
	store := openIsolatedStore(t, h.projectID)
	engine, err := sync.NewEngine(sync.Config{Store: store, Credential: wrong, HTTPClient: h.server.Client()})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	_, err = engine.Sync(ctx)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
		t.Fatalf("Sync() = %v, want unauthorized", err)
	}
}

type convergenceHarness struct {
	projectID   string
	credential  crypto.BundledClientCredential
	server      *httptest.Server
	serverStore *syncserver.Store
	projectKey  [32]byte
	keyGen      int
}

func newConvergenceHarness(t *testing.T) *convergenceHarness {
	t.Helper()
	ctx := context.Background()
	serverStore, err := syncserver.OpenStore(filepath.Join(t.TempDir(), "relay.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore relay: %v", err)
	}
	t.Cleanup(func() { _ = serverStore.Close() })

	projectID := "proj_convergence_test"
	adminSecret := "admin-secret"
	_, adminKey, err := serverStore.CreateAccount(ctx, adminSecret)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	token, tokenSecret, err := serverStore.MintConnectionToken(ctx, adminKey, adminSecret, "client", projectID)
	if err != nil {
		t.Fatalf("MintConnectionToken: %v", err)
	}
	master, err := crypto.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	ring := crypto.NewProjectKeyRing(master, projectID)
	projectKey, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey: %v", err)
	}
	server := httptest.NewServer(syncserver.NewServer(serverStore).Handler())
	t.Cleanup(server.Close)
	return &convergenceHarness{
		projectID: projectID,
		credential: crypto.BundledClientCredential{
			Endpoint: server.URL, ConnectionToken: token.TokenID + ":" + tokenSecret,
			ProjectID: projectID, KeyGeneration: ring.WriteGeneration, ProjectKey: projectKey[:],
		},
		server: server, serverStore: serverStore, projectKey: projectKey, keyGen: ring.WriteGeneration,
	}
}

func openIsolatedStore(t *testing.T, projectID string) *state.Store {
	t.Helper()
	store, err := state.OpenStore(filepath.Join(t.TempDir(), "loaf.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mustExec(t, store, `INSERT INTO projects (id, identity_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`, projectID, projectID+"-hash", now, now)
	return store
}

func newEngine(t *testing.T, store *state.Store, h *convergenceHarness) *sync.Engine {
	return newEngineWithSkew(t, store, h, int64((24*time.Hour)/time.Millisecond))
}

func newEngineWithSkew(t *testing.T, store *state.Store, h *convergenceHarness, skewMS int64) *sync.Engine {
	t.Helper()
	engine, err := sync.NewEngine(sync.Config{Store: store, Credential: h.credential, HTTPClient: h.server.Client(), MaxHLCSkewMS: skewMS})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

func sealFactForHarness(t *testing.T, h *convergenceHarness, fact state.FactEnvelope) ([]byte, error) {
	t.Helper()
	env, err := crypto.EncryptFactEnvelope(h.projectKey, fact.ID, crypto.FactPlaintext{
		Kind: fact.Kind, Payload: fact.Payload, EnvID: fact.EnvID, Seq: fact.Seq,
		HLC: fact.HLC, EnvelopeV: fact.EnvelopeV, KeyGen: h.keyGen,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

func mustExec(t *testing.T, store *state.Store, query string, args ...any) {
	t.Helper()
	if _, err := store.DB().ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func TestPullRefAndVerificationFactsRefreshProjections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := newConvergenceHarness(t)
	publisher := openIsolatedStore(t, h.projectID)
	receiver := openIsolatedStore(t, h.projectID)

	now := time.Date(2026, 8, 26, 19, 0, 0, 0, time.UTC)
	refPayload, err := json.Marshal(state.CoreEventPayload{
		SubjectKind:  "ref",
		SubjectID:    "bmap-sync-1",
		Backend:      "linear",
		EntityKind:   "issue",
		EntityID:     "issue-sync-1",
		ExternalKind: "issue",
		ExternalID:   "LOAF-62",
		ExternalURL:  "https://linear.app/loaf/issue/LOAF-62",
		SyncStatus:   "linked",
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal ref payload: %v", err)
	}
	verifyPayload, err := json.Marshal(state.CoreEventPayload{
		SubjectKind:  "verification",
		SubjectID:    "wcr-sync-1",
		Provider:     "branch",
		ProviderRef:  "issue/loaf-62",
		ReceiptKind:  "pr",
		ReceiptValue: "https://github.com/levifig/loaf/pull/204",
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal verification payload: %v", err)
	}

	if _, err := state.AppendFact(ctx, publisher, state.AppendFactInput{
		ProjectID: h.projectID, Kind: state.FactKindRefRegistered,
		Payload: string(refPayload), EnvID: "env-pub", Now: now,
	}); err != nil {
		t.Fatalf("append ref: %v", err)
	}
	if _, err := state.AppendFact(ctx, publisher, state.AppendFactInput{
		ProjectID: h.projectID, Kind: state.FactKindVerificationRecorded,
		Payload: string(verifyPayload), EnvID: "env-pub", Now: now.Add(time.Second),
	}); err != nil {
		t.Fatalf("append verification: %v", err)
	}

	if _, err := newEngine(t, publisher, h).Sync(ctx); err != nil {
		t.Fatalf("sync publisher: %v", err)
	}
	if _, err := newEngine(t, receiver, h).Sync(ctx); err != nil {
		t.Fatalf("sync receiver: %v", err)
	}

	var externalID, receiptValue string
	if err := receiver.DB().QueryRowContext(ctx, `SELECT external_id FROM backend_mappings WHERE id = ?`, "bmap-sync-1").Scan(&externalID); err != nil {
		t.Fatalf("receiver backend mapping: %v", err)
	}
	if externalID != "LOAF-62" {
		t.Fatalf("receiver external_id = %q, want LOAF-62", externalID)
	}
	if err := receiver.DB().QueryRowContext(ctx, `SELECT receipt_value FROM work_contract_receipts WHERE id = ?`, "wcr-sync-1").Scan(&receiptValue); err != nil {
		t.Fatalf("receiver verification receipt: %v", err)
	}
	if receiptValue != "https://github.com/levifig/loaf/pull/204" {
		t.Fatalf("receiver receipt_value = %q", receiptValue)
	}
}
